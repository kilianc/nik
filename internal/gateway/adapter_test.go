package gateway

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/messaging"
)

// fakeReceiver records what the adapter feeds into the messaging service
type fakeReceiver struct {
	mu            sync.Mutex
	messages      []messaging.InboundMessage
	conversations []messaging.Conversation
	existing      map[string]bool
}

func newFakeReceiver() *fakeReceiver {
	return &fakeReceiver{existing: map[string]bool{}}
}

func (r *fakeReceiver) MessageExists(_ context.Context, _ string, externalMessageID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.existing[externalMessageID], nil
}

func (r *fakeReceiver) ReceiveConversation(_ context.Context, conv messaging.Conversation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.conversations = append(r.conversations, conv)

	return nil
}

func (r *fakeReceiver) ReceiveMessage(_ context.Context, msg messaging.InboundMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.messages = append(r.messages, msg)
	r.existing[msg.ExternalMessageID] = true

	return nil
}

func (r *fakeReceiver) OnHistorySyncComplete(_ context.Context, _ string) error { return nil }

func (r *fakeReceiver) received() []messaging.InboundMessage {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]messaging.InboundMessage(nil), r.messages...)
}

func (r *fakeReceiver) convs() []messaging.Conversation {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]messaging.Conversation(nil), r.conversations...)
}

func startAdapter(t *testing.T) (*Adapter, *fakeGateway, *fakeReceiver) {
	t.Helper()

	gw := newFakeGateway(t)
	cfg := config.Default(t.TempDir())

	a := NewAdapter(cfg, gw.url(), "test-token", "test", testKey(t))

	recv := newFakeReceiver()

	err := a.Start(context.Background(), recv)
	if err != nil {
		t.Fatalf("start adapter: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { _ = a.Connect(ctx) }()

	waitUntil(t, "handshake", func() bool { return a.SelfJID() != "" })

	return a, gw, recv
}

func TestAdapterImplementsMessagingPlatform(t *testing.T) {
	var _ messaging.MessagingPlatform = (*Adapter)(nil)
}

func TestAdapterDeliversInboundMessage(t *testing.T) {
	_, gw, recv := startAdapter(t)

	sent := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)

	gw.push(t, msgIn{
		Chat:    "14155551234@s.whatsapp.net",
		Sender:  "14155551234@s.whatsapp.net",
		WaID:    "wa-1",
		SentAt:  sent,
		IsGroup: false,
	}, msgContent{
		Kind:     kindText,
		Body:     "what's for dinner?",
		Mentions: []string{"16502811468@s.whatsapp.net"},
		Quote:    &quoteRef{StanzaID: "wa-0", Participant: "14155551234@s.whatsapp.net", Body: "earlier"},
	})

	waitUntil(t, "message received", func() bool { return len(recv.received()) == 1 })

	got := recv.received()[0]

	if got.Platform != platform {
		t.Errorf("platform = %q", got.Platform)
	}
	if got.Kind != kindText || got.Body != "what's for dinner?" {
		t.Errorf("kind/body = %q/%q", got.Kind, got.Body)
	}
	if !got.SentAt.Equal(sent) {
		t.Errorf("sent_at = %v, want %v", got.SentAt, sent)
	}
	if got.ContextStanzaID != "wa-0" {
		t.Errorf("quote stanza = %q", got.ContextStanzaID)
	}
	if len(got.ContextMentionedIDs) != 1 {
		t.Errorf("mentions = %v", got.ContextMentionedIDs)
	}
	if got.IsFromMe {
		t.Error("inbound message marked as from me")
	}

	// the conversation was announced before the message
	convs := recv.convs()
	if len(convs) == 0 || convs[0].Kind != convDM {
		t.Fatalf("conversations = %+v", convs)
	}
}

func TestAdapterDedupsReplayedMessages(t *testing.T) {
	_, gw, recv := startAdapter(t)

	msg := msgIn{
		Chat:   "14155551234@s.whatsapp.net",
		Sender: "14155551234@s.whatsapp.net",
		WaID:   "wa-dup",
	}
	content := msgContent{Kind: kindText, Body: "once"}

	gw.push(t, msg, content)
	waitUntil(t, "first delivery", func() bool { return len(recv.received()) == 1 })

	// the gateway is at-least-once: a reconnect replays unacked rows
	gw.push(t, msg, content)

	time.Sleep(100 * time.Millisecond)

	if n := len(recv.received()); n != 1 {
		t.Errorf("message delivered %d times, want 1", n)
	}
}

func TestAdapterCollectsAttachment(t *testing.T) {
	a, gw, recv := startAdapter(t)

	photo := []byte("jpeg-bytes")

	sealed, err := sealTo(photo, a.client.pub)
	if err != nil {
		t.Fatalf("seal photo: %v", err)
	}

	gw.mu.Lock()
	gw.blobs["blob-photo"] = sealed
	gw.mu.Unlock()

	gw.push(t, msgIn{
		Chat:   "14155551234@s.whatsapp.net",
		Sender: "14155551234@s.whatsapp.net",
		WaID:   "wa-photo",
	}, msgContent{
		Kind:     kindImage,
		Body:     "look at this",
		MimeType: "image/jpeg",
		Media:    &mediaRef{DownloadID: "blob-photo", MMSType: "image", SizeBytes: int64(len(photo))},
	})

	waitUntil(t, "photo received", func() bool { return len(recv.received()) == 1 })

	got := recv.received()[0]

	if got.LocalPath == "" {
		t.Fatal("no local path on the delivered photo")
	}
	if got.MediaSizeBytes != int64(len(photo)) {
		t.Errorf("media size = %d, want %d", got.MediaSizeBytes, len(photo))
	}

	// the plaintext landed in the media folder
	data, err := os.ReadFile(filepath.Join(a.cfg.Home, got.LocalPath))
	if err != nil {
		t.Fatalf("read collected attachment: %v", err)
	}
	if string(data) != string(photo) {
		t.Errorf("attachment = %q, want %q", data, photo)
	}
}

func TestAdapterAttachmentFailureKeepsMessage(t *testing.T) {
	_, gw, recv := startAdapter(t)

	gw.push(t, msgIn{
		Chat:   "14155551234@s.whatsapp.net",
		Sender: "14155551234@s.whatsapp.net",
		WaID:   "wa-lost",
	}, msgContent{
		Kind:     kindImage,
		Body:     "a photo",
		MimeType: "image/jpeg",
		Media:    &mediaRef{DownloadID: "no-such-blob", MMSType: "image"},
	})

	waitUntil(t, "caption received", func() bool { return len(recv.received()) == 1 })

	got := recv.received()[0]

	if got.Body != "a photo" {
		t.Errorf("body = %q", got.Body)
	}
	if got.LocalPath != "" {
		t.Errorf("local path = %q for an uncollectable attachment", got.LocalPath)
	}
}

func TestAdapterOutboundVerbs(t *testing.T) {
	a, gw, _ := startAdapter(t)
	ctx := context.Background()

	const chat = "14155551234@s.whatsapp.net"

	_, err := a.Reply(ctx, chat, "pasta tonight", &messaging.QuoteTarget{
		ExternalMessageID: "wa-q",
		ExternalSenderID:  chat,
		Body:              "what's for dinner?",
		Kind:              kindText,
	})
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	_, err = a.React(ctx, chat, "wa-1", chat, "🦊")
	if err != nil {
		t.Fatalf("react: %v", err)
	}

	err = a.StartTyping(ctx, chat)
	if err != nil {
		t.Fatalf("start typing: %v", err)
	}

	err = a.MarkRead(ctx, []messaging.InboundMessage{
		{ExternalConversationID: chat, ExternalMessageID: "wa-1", ExternalSenderID: chat},
		{ExternalConversationID: chat, ExternalMessageID: "wa-2", ExternalSenderID: chat},
	})
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}

	waitUntil(t, "all verbs at the gateway", func() bool { return len(gw.sent()) >= 4 })

	var sawReply, sawReact, sawTyping bool
	var reads []readOut

	for _, env := range gw.sent() {
		switch env.Type {
		case typeMsgOut:
			out, err := decodePayload[msgOut](env)
			if err != nil {
				t.Fatalf("decode msg.out: %v", err)
			}
			if out.Quote == nil || out.Quote.StanzaID != "wa-q" {
				t.Errorf("reply quote = %+v", out.Quote)
			}

			sawReply = true
		case typeReactOut:
			sawReact = true
		case typeTypingOut:
			sawTyping = true
		case typeReadOut:
			r, err := decodePayload[readOut](env)
			if err != nil {
				t.Fatalf("decode read.out: %v", err)
			}

			reads = append(reads, r)
		}
	}

	if !sawReply || !sawReact || !sawTyping {
		t.Errorf("verbs seen: reply=%v react=%v typing=%v", sawReply, sawReact, sawTyping)
	}

	// two messages in one conversation batch into a single read receipt
	if len(reads) != 1 || len(reads[0].WaIDs) != 2 {
		t.Errorf("read receipts = %+v, want one with two ids", reads)
	}
}

func TestAdapterSendFileUploadsThenNames(t *testing.T) {
	a, gw, _ := startAdapter(t)
	ctx := context.Background()

	path := filepath.Join(t.TempDir(), "recipe.pdf")

	err := os.WriteFile(path, []byte("%PDF-fake"), 0o600)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	out, err := a.SendFile(ctx, "14155551234@s.whatsapp.net", path, "tonight")
	if err != nil {
		t.Fatalf("send file: %v", err)
	}
	if out.Kind != kindDocument {
		t.Errorf("kind = %q", out.Kind)
	}

	waitUntil(t, "media.out", func() bool {
		for _, env := range gw.sent() {
			if env.Type == typeMediaOut {
				return true
			}
		}

		return false
	})

	gw.mu.Lock()
	uploaded := string(gw.uploads["mh_test"])
	gw.mu.Unlock()

	if uploaded != "%PDF-fake" {
		t.Errorf("uploaded bytes = %q", uploaded)
	}

	for _, env := range gw.sent() {
		if env.Type != typeMediaOut {
			continue
		}

		m, err := decodePayload[mediaOut](env)
		if err != nil {
			t.Fatalf("decode media.out: %v", err)
		}
		if m.Handle != "mh_test" || m.Caption != "tonight" {
			t.Errorf("media.out = %+v", m)
		}
	}
}

func TestAdapterSetPresenceIsNoOp(t *testing.T) {
	a, gw, _ := startAdapter(t)

	err := a.SetPresence(context.Background(), true)
	if err != nil {
		t.Fatalf("set presence: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	for _, env := range gw.sent() {
		if env.Type != typeAck {
			t.Errorf("presence produced a %s envelope — it belongs to the shared number", env.Type)
		}
	}
}

func TestEnabled(t *testing.T) {
	cfg := config.Default(t.TempDir())
	store := newMemSecrets()

	if Enabled(cfg, store) {
		t.Error("enabled with no url and no token")
	}

	cfg.Gateway.URL = "wss://nik-gw.example.com/v1/agent"
	if Enabled(cfg, store) {
		t.Error("enabled with no token")
	}

	err := store.Set(tokenSecretName, "nik_token")
	if err != nil {
		t.Fatalf("set token: %v", err)
	}
	if !Enabled(cfg, store) {
		t.Error("not enabled with url and token both present")
	}

	cfg.Gateway.URL = ""
	if Enabled(cfg, store) {
		t.Error("enabled with a token but no url")
	}
}

// Probe is what setup runs on the token a person just pasted. It must tell
// apart the three answers a person acts on differently: yes, wrong token,
// and can't reach.
func TestProbe(t *testing.T) {
	gw := newFakeGateway(t)
	ctx := context.Background()

	if keep, err := Probe(ctx, gw.url(), "test-token", testKey(t)); err != nil || keep == "" {
		t.Fatalf("good token: keep=%q err=%v", keep, err)
	}
	if _, err := Probe(ctx, gw.url(), "nik_wrong", testKey(t)); !errors.Is(err, ErrAuthRejected) {
		t.Fatalf("wrong token = %v, want ErrAuthRejected", err)
	}
	_, err := Probe(ctx, "ws://127.0.0.1:1/v1/agent", "test-token", testKey(t))
	if err == nil || errors.Is(err, ErrAuthRejected) {
		t.Fatalf("unreachable = %v, want a dial error", err)
	}
}

// Rotation end to end: the install token dies on first connect, the store
// holds what the gateway handed back, and a daemon booting with THAT token
// gets rotated again and keeps working — twice over is the case that
// catches "stored the typed one" bugs.
func TestTokenRotationPersists(t *testing.T) {
	gw := newFakeGateway(t)
	ctx := context.Background()
	store := newMemSecrets()

	// nik connect / setup: probe with the install token.
	if err := ProbeWithStore(ctx, gw.url(), "test-token", store); err != nil {
		t.Fatal(err)
	}
	stored, _ := store.Get(tokenSecretName)
	if stored == "test-token" || stored == "" {
		t.Fatalf("store holds %q — the install token, not the rotated one", stored)
	}
	// The install token is dead now.
	if _, err := Probe(ctx, gw.url(), "test-token", testKey(t)); !errors.Is(err, ErrAuthRejected) {
		t.Fatalf("install token still accepted after rotation: %v", err)
	}

	// Boot: FromConfig dials with the stored token, gets rotated again,
	// and persists the newer one.
	cfg := &config.Config{Gateway: config.GatewayConfig{URL: gw.url()}}
	a, err := FromConfig(cfg, store, "boot")
	if err != nil {
		t.Fatal(err)
	}
	bctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = a.Connect(bctx) }()
	select {
	case <-a.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("daemon never got hello.ack with the rotated token")
	}
	waitUntil(t, "second rotation persisted", func() bool {
		now, _ := store.Get(tokenSecretName)
		return now != stored && now != ""
	})
}

// Another process (nik connect, a second daemon) rotates the token while
// this daemon is up: its next dial is refused, but the store already holds
// the live token — it must adopt that and carry on, not die.
func TestReconnectAdoptsTokenRotatedElsewhere(t *testing.T) {
	gw := newFakeGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := newMemSecrets()
	if err := store.Set(tokenSecretName, "test-token"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Gateway: config.GatewayConfig{URL: gw.url()}}
	a, err := FromConfig(cfg, store, "daemon")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.Connect(ctx) }()
	select {
	case <-a.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("no first hello.ack")
	}

	// The gateway restarts (drops the daemon's socket) and, before the daemon
	// gets back in, another process — nik connect, a second daemon — dials
	// with the live token, gets it rotated, and writes the new one into the
	// shared store. The daemon's in-memory token is now stale: its reconnect
	// is refused, and it must adopt what the store holds.
	live, _ := store.Get(tokenSecretName)
	if err := ProbeWithStore(ctx, gw.url(), live, store); err != nil {
		t.Fatal(err)
	}
	// The daemon still holds `live` in memory, which the probe just retired.
	// Its next reconnect will be refused; drop it so that happens now.
	gw.dropAll()

	select {
	case err := <-done:
		t.Fatalf("daemon died instead of adopting the stored token: %v", err)
	case <-time.After(3 * time.Second):
		// still running — good. And it must have re-rotated after adopting.
	}
	waitUntil(t, "daemon reconnected with the adopted token", func() bool {
		gw.mu.Lock()
		defer gw.mu.Unlock()
		return gw.rotations >= 3
	})
}
