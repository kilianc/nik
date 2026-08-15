package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/messaging"
)

// Adapter runs nik against the nik-saas gateway instead of a local whatsapp
// session: no SIM, no QR, no phone. it reports platform "whatsapp" on purpose —
// conversations, contacts and skills are all keyed on that, and the messages
// really are whatsapp messages, just routed through a number we don't own.

const platform = "whatsapp"

type Adapter struct {
	cfg    *config.Config
	client *client

	receiver messaging.MessageReceiver
}

func NewAdapter(cfg *config.Config, url, token, name string, priv *[keySize]byte) *Adapter {
	return &Adapter{cfg: cfg, client: newClient(url, token, name, priv)}
}

func (a *Adapter) Platform() string { return platform }

func (a *Adapter) Start(_ context.Context, receiver messaging.MessageReceiver) error {
	a.receiver = receiver

	a.client.onMessage = a.handleMessage
	a.client.onConversation = a.handleConversation
	a.client.onReady = a.handleReady

	return nil
}

// Connect runs the gateway session until ctx ends. unlike the whatsapp client
// there is nothing to pair: the install token is the whole credential.
func (a *Adapter) Connect(ctx context.Context) error {
	return a.client.run(ctx)
}

// Ready is closed once the first hello.ack lands — the gateway accepted the
// token and this daemon is live. Boot blocks on it: connecting is a boot
// step, not a background hope.
func (a *Adapter) Ready() <-chan struct{} { return a.client.ready }

func (a *Adapter) Stop(_ context.Context) error {
	a.client.close()

	return nil
}

// SelfJID is the central number, learned from hello.ack
func (a *Adapter) SelfJID() string { return a.client.SelfJID() }

func (a *Adapter) Reply(ctx context.Context, externalConversationID string, body string, quote *messaging.QuoteTarget) (messaging.OutboundMessage, error) {
	out := msgOut{Chat: externalConversationID, Text: body}
	if quote != nil {
		out.Quote = &quoteRef{
			StanzaID:    quote.ExternalMessageID,
			Participant: quote.ExternalSenderID,
			Body:        quote.Body,
			Kind:        quote.Kind,
		}
	}

	err := a.client.send(ctx, typeMsgOut, out)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("send reply: %w", err)
	}

	return a.sent(kindText, body, ""), nil
}

func (a *Adapter) SendFile(ctx context.Context, externalConversationID string, filePath string, caption string) (messaging.OutboundMessage, error) {
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	err := a.sendUpload(ctx, externalConversationID, filePath, mimeType, caption, mediaDocument)
	if err != nil {
		return messaging.OutboundMessage{}, err
	}

	return a.sent(kindFor(mimeType), caption, mimeType), nil
}

func (a *Adapter) SendVoiceNote(ctx context.Context, externalConversationID string, audioPath string) (messaging.OutboundMessage, error) {
	const mimeType = "audio/ogg; codecs=opus"

	err := a.sendUpload(ctx, externalConversationID, audioPath, mimeType, "", mediaVoice)
	if err != nil {
		return messaging.OutboundMessage{}, err
	}

	return a.sent(kindAudio, "", mimeType), nil
}

func (a *Adapter) React(ctx context.Context, externalConversationID string, externalMessageID string, externalSenderID string, emoji string) (messaging.OutboundMessage, error) {
	err := a.client.send(ctx, typeReactOut, reactOut{
		Chat:        externalConversationID,
		WaID:        externalMessageID,
		Participant: externalSenderID,
		Emoji:       emoji,
	})
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("send reaction: %w", err)
	}

	return a.sent(kindReaction, emoji, ""), nil
}

// SetPresence is a no-op. presence belongs to the central number, which every
// family shares — one nik going quiet must not mark nik offline for everyone.
// the gateway owns it and exposes no verb for it.
func (a *Adapter) SetPresence(_ context.Context, _ bool) error { return nil }

func (a *Adapter) StartTyping(ctx context.Context, externalConversationID string) error {
	return a.client.send(ctx, typeTypingOut, typingOut{Chat: externalConversationID, State: "start"})
}

func (a *Adapter) StopTyping(ctx context.Context, externalConversationID string) error {
	return a.client.send(ctx, typeTypingOut, typingOut{Chat: externalConversationID, State: "stop"})
}

// MarkRead batches by conversation: the wire takes many message ids per
// conversation, and a family reading a burst of group messages should not
// produce one envelope each.
func (a *Adapter) MarkRead(ctx context.Context, refs []messaging.InboundMessage) error {
	byChat := map[string]*readOut{}
	order := []string{}

	for _, ref := range refs {
		out, ok := byChat[ref.ExternalConversationID]
		if !ok {
			out = &readOut{Chat: ref.ExternalConversationID, Participant: ref.ExternalSenderID}
			byChat[ref.ExternalConversationID] = out
			order = append(order, ref.ExternalConversationID)
		}

		out.WaIDs = append(out.WaIDs, ref.ExternalMessageID)
	}

	for _, chat := range order {
		err := a.client.send(ctx, typeReadOut, *byChat[chat])
		if err != nil {
			return fmt.Errorf("send read receipt: %w", err)
		}
	}

	return nil
}

func (a *Adapter) handleReady(_ context.Context, ack helloAck) {
	slog.Info("gateway ready", "pkg", "gateway", "number", ack.Number)
}

func (a *Adapter) handleConversation(ctx context.Context, conv convIn, content convContent) error {
	if a.receiver == nil {
		return nil
	}

	now := time.Now()

	return a.receiver.ReceiveConversation(ctx, messaging.Conversation{
		Platform:               platform,
		ExternalConversationID: conv.Chat,
		Kind:                   content.Kind,
		Title:                  content.Title,
		Topic:                  optional(content.Topic),
		IsAnnounce:             optional(content.IsAnnounce),
		IsLocked:               optional(content.IsLocked),
		OwnerExternalID:        optional(content.Owner),
		ParticipantExternalIDs: content.Participants,
		LastMessageAt:          now,
	})
}

func (a *Adapter) handleMessage(ctx context.Context, msg msgIn, content msgContent) error {
	if a.receiver == nil {
		return nil
	}

	// the gateway is at-least-once, and a reconnect replays anything unacked
	exists, err := a.receiver.MessageExists(ctx, platform, msg.WaID)
	if err == nil && exists {
		slog.Debug("skip existing message", "pkg", "gateway", "msg_id", msg.WaID)

		return nil
	}

	convKind := convDM
	if msg.IsGroup {
		convKind = convGroup
	}

	sentAt := msg.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now()
	}

	err = a.receiver.ReceiveConversation(ctx, messaging.Conversation{
		Platform:               platform,
		ExternalConversationID: msg.Chat,
		Kind:                   convKind,
		LastMessageAt:          sentAt,
	})
	if err != nil {
		return err
	}

	inbound := messaging.InboundMessage{
		Platform:               platform,
		ExternalConversationID: msg.Chat,
		ExternalMessageID:      msg.WaID,
		ExternalSenderID:       msg.Sender,
		ExternalSenderIDs:      []string{msg.Sender},
		Kind:                   content.Kind,
		Body:                   content.Body,
		MimeType:               content.MimeType,
		SentAt:                 sentAt,
		IsGroup:                msg.IsGroup,
		IsEdit:                 content.IsEdit,
		EditTargetMessageID:    content.EditTarget,
		ContextIsForwarded:     content.Forwarded,
		ContextMentionedIDs:    content.Mentions,
		IsEphemeral:            content.Ephemeral,
		IsViewOnce:             content.ViewOnce,
	}

	if content.ForwardingScore != 0 {
		score := content.ForwardingScore
		inbound.ContextForwardingScore = &score
	}

	if content.Quote != nil {
		inbound.ContextStanzaID = content.Quote.StanzaID
		inbound.ContextParticipant = content.Quote.Participant
	}

	if content.Media != nil {
		a.collectAttachment(ctx, &inbound, content)
	}

	return a.receiver.ReceiveMessage(ctx, inbound)
}

// collectAttachment fetches the sealed attachment and writes the plaintext into
// nik's media folder. a failure costs the attachment, never the message: a
// caption with a missing photo still beats silence.
func (a *Adapter) collectAttachment(ctx context.Context, inbound *messaging.InboundMessage, content msgContent) {
	data, err := a.client.fetchAttachment(ctx, content.Media.DownloadID)
	if err != nil {
		slog.Warn("fetch attachment", "pkg", "gateway", "error", err, "msg_id", inbound.ExternalMessageID)

		return
	}

	mediaID := id.V7()
	filename := mediaID + extensionFor(content.MimeType)
	dir := a.cfg.MediaPath()

	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		slog.Warn("create media dir", "pkg", "gateway", "error", err)

		return
	}

	err = os.WriteFile(filepath.Join(dir, filename), data, 0o600)
	if err != nil {
		slog.Warn("write attachment", "pkg", "gateway", "error", err)

		return
	}

	inbound.LocalPath = filepath.Join("media", filename)
	inbound.MediaID = mediaID
	inbound.MediaSizeBytes = int64(len(data))
}

func (a *Adapter) sendUpload(ctx context.Context, chat, path, mimeType, caption, kind string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	handle, err := a.client.uploadMedia(ctx, data, mimeType, filepath.Base(path))
	if err != nil {
		return err
	}

	err = a.client.send(ctx, typeMediaOut, mediaOut{
		Chat:    chat,
		Handle:  handle,
		Kind:    kind,
		Caption: caption,
	})
	if err != nil {
		return fmt.Errorf("send media: %w", err)
	}

	return nil
}

// sent synthesizes the record of an outbound message. the gateway acks but
// does not report whatsapp's assigned id or timestamp, so nik keeps its own —
// enough to store the message and show it in a conversation.
func (a *Adapter) sent(kind, body, mimeType string) messaging.OutboundMessage {
	sender := a.client.SelfJID()
	if sender == "" {
		sender = db.NikContactID
	}

	return messaging.OutboundMessage{
		ExternalMessageID: id.V7(),
		ExternalSenderID:  sender,
		SentAt:            time.Now(),
		Kind:              kind,
		Body:              body,
		MimeType:          mimeType,
	}
}

func kindFor(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return kindImage
	case strings.HasPrefix(mimeType, "video/"):
		return kindVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return kindAudio
	default:
		return kindDocument
	}
}

func extensionFor(mimeType string) string {
	base, _, _ := strings.Cut(mimeType, ";")

	exts, err := mime.ExtensionsByType(strings.TrimSpace(base))
	if err != nil || len(exts) == 0 {
		return ".bin"
	}

	return exts[0]
}

func optional[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}

	return &v
}

// DefaultURL is the production gateway. Setup writes it when config names
// no other; a dev install overrides it in config.yaml before running setup.
const DefaultURL = "wss://nik-gw.ciuffolo.com/v1/agent"

// Probe checks a token the way boot does: dial, hello, wait for the first
// ack, hang up — and returns the token to keep, which is the rotated one
// the ack carried (the gateway retires whatever we dialed with). A rejected
// token comes back as ErrAuthRejected so callers can say "wrong token"
// rather than "network"; anything else is the dial or handshake error.
//
// The key is real, not throwaway: hello registers it, and it must be the
// same key the daemon will later hold, or messages sealed in between would
// be unreadable. Pass the store's key.
func Probe(ctx context.Context, url, token string, priv *[keySize]byte) (keep string, err error) {
	c := newClient(url, token, "setup", priv)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.session(ctx) }()
	select {
	case <-c.ready:
		cancel()
		c.mu.Lock()
		keep = c.token
		c.mu.Unlock()
		return keep, nil
	case err := <-done:
		if err == nil {
			return "", errors.New("gateway closed before saying hello")
		}
		return "", err
	case <-ctx.Done():
		return "", errors.New("no answer from the gateway within 20s")
	}
}

// ProbeWithStore is Probe for callers holding a secret store: it uses (or
// creates) the store's agent key and persists whatever token the gateway
// hands back, so the daemon that boots next dials with the right pair.
func ProbeWithStore(ctx context.Context, url, token string, store secretStore) error {
	priv, err := loadOrCreateKey(store)
	if err != nil {
		return err
	}
	keep, err := Probe(ctx, url, token, priv)
	if err != nil {
		return err
	}
	return store.Set(tokenSecretName, keep)
}

// ErrAuthRejected is what Probe returns for a token the gateway refused.
var ErrAuthRejected = errAuthRejected

// tokenSecretName holds the install token from the nik-saas dashboard
const tokenSecretName = "gateway_token"

// Enabled reports whether the gateway is configured: a url in config and an
// install token in the secret store. the daemon refuses to start without both
// — the gateway is nik's only route to whatsapp.
func Enabled(cfg *config.Config, store secretStore) bool {
	if cfg.Gateway.URL == "" {
		return false
	}

	token, err := store.Get(tokenSecretName)

	return err == nil && token != ""
}

// FromConfig builds the adapter from config and the secret store, creating the
// agent key on first run.
func FromConfig(cfg *config.Config, store secretStore, name string) (*Adapter, error) {
	token, err := store.Get(tokenSecretName)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", tokenSecretName, err)
	}

	priv, err := loadOrCreateKey(store)
	if err != nil {
		return nil, err
	}

	a := NewAdapter(cfg, cfg.Gateway.URL, token, name, priv)
	// The gateway rotates the token on every connect. Persist each one: the
	// token in the store is then always one nobody typed, and the install
	// code from the dashboard's one-liner is dead after first use.
	a.client.onToken = func(fresh string) {
		if err := store.Set(tokenSecretName, fresh); err != nil {
			slog.Error("persist rotated gateway token", "pkg", "gateway", "error", err)
		}
	}
	a.client.reloadToken = func() (string, error) { return store.Get(tokenSecretName) }
	return a, nil
}
