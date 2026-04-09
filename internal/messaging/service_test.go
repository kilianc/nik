package messaging

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
)

func TestReceiveMessageFromMeUsesNikContactID(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	err = svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation-1@s.whatsapp.net",
		ExternalMessageID:      "msg-1",
		ExternalSenderID:       "11111@s.whatsapp.net",
		Kind:                   "text",
		Body:                   "hello from nik",
		SentAt:                 time.Now(),
		IsFromMe:               true,
	})
	if err != nil {
		t.Fatalf("receive message: %v", err)
	}

	msg, err := db.MessageGet(ctx, conn, db.MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "msg-1",
	})
	if err != nil {
		t.Fatalf("get message: %v", err)
	}

	if msg.ContactID != contacts.NikContactID {
		t.Fatalf("expected contact id %s, got %s", contacts.NikContactID, msg.ContactID)
	}
}

func TestReceiveMessageFailsWithoutExternalSenderID(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	err = svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation-2@s.whatsapp.net",
		ExternalMessageID:      "msg-2",
		Kind:                   "text",
		Body:                   "missing sender",
		SentAt:                 time.Now(),
	})
	if err == nil {
		t.Fatalf("expected error for missing external sender id")
	}
	if !strings.Contains(err.Error(), "empty external_sender_id") {
		t.Fatalf("expected empty external_sender_id error, got %v", err)
	}
}

type failingContactService struct{}

func (f *failingContactService) EnsureContactForMessage(_ context.Context, _ string, _ []string, _ bool, _ time.Time) (string, error) {
	return "", errors.New("resolve failed")
}

type fakeReceiver struct{}

func (fakeReceiver) MessageExists(context.Context, string, string) (bool, error) { return false, nil }
func (fakeReceiver) ReceiveConversation(context.Context, Conversation) error     { return nil }
func (fakeReceiver) ReceiveMessage(context.Context, InboundMessage) error        { return nil }
func (fakeReceiver) OnHistorySyncComplete(context.Context, string) error         { return nil }

func TestAdapterContractsCompileAndExposePlatformName(t *testing.T) {
	var _ MessageReceiver = (*fakeReceiver)(nil)
	var _ MessagingPlatform = (*mockPlatform)(nil)

	p := &mockPlatform{platform: "whatsapp"}
	if p.Platform() != "whatsapp" {
		t.Fatalf("expected platform name whatsapp, got %q", p.Platform())
	}
}

func TestReceiveMessageFailsWhenContactResolutionFails(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	svc := NewService(&config.Config{}, conn, &failingContactService{})

	err = svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation-3@s.whatsapp.net",
		ExternalMessageID:      "msg-3",
		ExternalSenderID:       "33333@s.whatsapp.net",
		Kind:                   "text",
		Body:                   "fail contact",
		SentAt:                 time.Now(),
	})
	if err == nil {
		t.Fatalf("expected error for contact resolution failure")
	}
	if !strings.Contains(err.Error(), "resolve contact") {
		t.Fatalf("expected resolve contact error, got %v", err)
	}
}

func TestSenderLabelsResolvesContactName(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = db.ContactUpsert(ctx, conn, db.ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "44444@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "44444",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	err = svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation-4@s.whatsapp.net",
		ExternalMessageID:      "msg-4",
		ExternalSenderID:       "44444@s.whatsapp.net",
		Kind:                   "text",
		Body:                   "hello",
		SentAt:                 time.Now(),
	})
	if err != nil {
		t.Fatalf("receive message: %v", err)
	}

	msg, err := db.MessageGet(ctx, conn, db.MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "msg-4",
	})
	if err != nil {
		t.Fatalf("get message: %v", err)
	}

	_, msgs, err := svc.ConversationWithMessages(ctx, msg.ConversationID, 10)
	if err != nil {
		t.Fatalf("conversation with messages: %v", err)
	}

	labels := svc.SenderLabels(ctx, msgs)
	line := formatMessageLine(msgs[0], labels[msgs[0].ID])
	if !strings.Contains(line, "Alice: hello") {
		t.Fatalf("expected resolved contact name in output, got %q", line)
	}
}

type mockPlatform struct {
	platform           string
	startTypingCalls   int
	stopTypingCalls    int
	replyCalls         int
	sendFileCalls      int
	sendVoiceNoteCalls int
	markReadCalls      int
	setPresenceCalls   int
	lastPresenceOnline bool
	lastReadRefs       []InboundMessage
	lastQuote          *QuoteTarget
	outbound           OutboundMessage
	fileOutbound       OutboundMessage
	voiceNoteOutbound  OutboundMessage
}

func (m *mockPlatform) Platform() string { return m.platform }
func (m *mockPlatform) Start(_ context.Context, _ MessageReceiver) error {
	return nil
}
func (m *mockPlatform) Stop(_ context.Context) error { return nil }
func (m *mockPlatform) Reply(_ context.Context, _ string, _ string, quote *QuoteTarget) (OutboundMessage, error) {
	m.replyCalls++
	m.lastQuote = quote
	return m.outbound, nil
}
func (m *mockPlatform) SendFile(_ context.Context, _ string, _ string, _ string) (OutboundMessage, error) {
	m.sendFileCalls++
	return m.fileOutbound, nil
}
func (m *mockPlatform) SendVoiceNote(_ context.Context, _ string, _ string) (OutboundMessage, error) {
	m.sendVoiceNoteCalls++
	return m.voiceNoteOutbound, nil
}
func (m *mockPlatform) React(_ context.Context, _, _, _, _ string) (OutboundMessage, error) {
	return OutboundMessage{}, nil
}
func (m *mockPlatform) SetPresence(_ context.Context, available bool) error {
	m.setPresenceCalls++
	m.lastPresenceOnline = available
	return nil
}
func (m *mockPlatform) StartTyping(_ context.Context, _ string) error {
	m.startTypingCalls++
	return nil
}
func (m *mockPlatform) StopTyping(_ context.Context, _ string) error {
	m.stopTypingCalls++
	return nil
}
func (m *mockPlatform) MarkRead(_ context.Context, refs []InboundMessage) error {
	m.markReadCalls++
	m.lastReadRefs = append([]InboundMessage(nil), refs...)
	return nil
}

func TestReplyPersistsOutboundImmediately(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)
	svc.replyDelay = func(string) time.Duration { return 0 }

	now := time.Now()
	err = db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	conversationID := conversation.ID

	platform := &mockPlatform{
		platform: "whatsapp",
		outbound: OutboundMessage{
			ExternalMessageID: "out-1",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "text",
			Body:              "hello",
		},
	}

	svc.RegisterPlatform(platform)

	err = svc.Reply(ctx, conversationID, "hello", nil)
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	msg, err := db.MessageGet(ctx, conn, db.MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "out-1",
	})
	if err != nil {
		t.Fatalf("get outbound message: %v", err)
	}

	if !msg.IsFromMe {
		t.Fatalf("expected outbound message is_from_me=true")
	}
	if msg.Body != "hello" {
		t.Fatalf("expected outbound body hello, got %q", msg.Body)
	}
	if platform.startTypingCalls != 1 || platform.stopTypingCalls != 1 {
		t.Fatalf("expected start/stop typing to be called once each, got start=%d stop=%d", platform.startTypingCalls, platform.stopTypingCalls)
	}
	if platform.replyCalls != 1 {
		t.Fatalf("expected one reply call, got %d", platform.replyCalls)
	}
}

func TestReplyWithQuoteTargetSetsContextStanza(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)
	svc.replyDelay = func(string) time.Duration { return 0 }

	now := time.Now()
	err = db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "group",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	platform := &mockPlatform{
		platform: "whatsapp",
		outbound: OutboundMessage{
			ExternalMessageID: "out-quote-1",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "text",
			Body:              "my reply",
		},
	}
	svc.RegisterPlatform(platform)

	quote := &QuoteTarget{
		ExternalMessageID: "original-msg-id",
		ExternalSenderID:  "sender@s.whatsapp.net",
		Body:              "the original message",
		Kind:              "text",
	}

	err = svc.Reply(ctx, conversation.ID, "my reply", quote)
	if err != nil {
		t.Fatalf("reply with quote: %v", err)
	}

	if platform.lastQuote == nil {
		t.Fatal("expected quote to be forwarded to platform")
	}
	if platform.lastQuote.ExternalMessageID != "original-msg-id" {
		t.Fatalf("expected quote external_message_id original-msg-id, got %q", platform.lastQuote.ExternalMessageID)
	}

	msg, err := db.MessageGet(ctx, conn, db.MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "out-quote-1",
	})
	if err != nil {
		t.Fatalf("get outbound message: %v", err)
	}

	if !msg.ContextStanzaID.Valid || msg.ContextStanzaID.String != "original-msg-id" {
		t.Fatalf("expected context_stanza_id=original-msg-id, got %v", msg.ContextStanzaID)
	}
	if !msg.ContextParticipant.Valid || msg.ContextParticipant.String != "sender@s.whatsapp.net" {
		t.Fatalf("expected context_participant=sender@s.whatsapp.net, got %v", msg.ContextParticipant)
	}
}

func TestSendFilePersistsOutbound(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)
	svc.replyDelay = func(string) time.Duration { return 0 }

	now := time.Now()
	err = db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "test-image-*.jpg")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.Write([]byte("fake image data"))
	tmpFile.Close()

	platform := &mockPlatform{
		platform: "whatsapp",
		fileOutbound: OutboundMessage{
			ExternalMessageID: "img-1",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "image",
			Body:              "check this out",
			MimeType:          "image/jpeg",
			LocalPath:         "abc123.jpg",
		},
	}
	svc.RegisterPlatform(platform)

	err = svc.SendFile(ctx, conversation.ID, tmpFile.Name(), "check this out")
	if err != nil {
		t.Fatalf("send file: %v", err)
	}

	msg, err := db.MessageGet(ctx, conn, db.MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "img-1",
	})
	if err != nil {
		t.Fatalf("get outbound image message: %v", err)
	}

	if !msg.IsFromMe {
		t.Fatalf("expected outbound message is_from_me=true")
	}
	if msg.Kind != "image" {
		t.Fatalf("expected kind image, got %q", msg.Kind)
	}
	if msg.Body != "check this out" {
		t.Fatalf("expected body 'check this out', got %q", msg.Body)
	}
	if platform.sendFileCalls != 1 {
		t.Fatalf("expected one send file call, got %d", platform.sendFileCalls)
	}
	if platform.startTypingCalls != 1 || platform.stopTypingCalls != 1 {
		t.Fatalf("expected start/stop typing once each, got start=%d stop=%d", platform.startTypingCalls, platform.stopTypingCalls)
	}
}

func TestSendVoiceNotePersistsOutbound(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	now := time.Now()
	err = db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "test-audio-*.ogg")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.Write([]byte("fake audio data"))
	tmpFile.Close()

	platform := &mockPlatform{
		platform: "whatsapp",
		voiceNoteOutbound: OutboundMessage{
			ExternalMessageID: "audio-1",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "audio",
			MimeType:          "audio/ogg; codecs=opus",
			LocalPath:         "abc123.ogg",
		},
	}
	svc.RegisterPlatform(platform)

	err = svc.SendVoiceNote(ctx, conversation.ID, tmpFile.Name(), "")
	if err != nil {
		t.Fatalf("send voice note: %v", err)
	}

	msg, err := db.MessageGet(ctx, conn, db.MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "audio-1",
	})
	if err != nil {
		t.Fatalf("get outbound audio message: %v", err)
	}

	if !msg.IsFromMe {
		t.Fatalf("expected outbound message is_from_me=true")
	}
	if msg.Kind != "audio" {
		t.Fatalf("expected kind audio, got %q", msg.Kind)
	}
	if platform.sendVoiceNoteCalls != 1 {
		t.Fatalf("expected one send voice note call, got %d", platform.sendVoiceNoteCalls)
	}
}

func TestSendVoiceNoteStoresTranscript(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	cfg := &config.Config{}
	cfg.Home = t.TempDir()
	svc := NewService(cfg, conn, contactsSvc)

	now := time.Now()
	err = db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "test-audio-*.ogg")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.Write([]byte("fake audio data for transcript test"))
	tmpFile.Close()

	platform := &mockPlatform{
		platform: "whatsapp",
		voiceNoteOutbound: OutboundMessage{
			ExternalMessageID: "audio-transcript-1",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "audio",
			MimeType:          "audio/ogg; codecs=opus",
			LocalPath:         "abc123.ogg",
		},
	}
	svc.RegisterPlatform(platform)

	ttsText := "Hey CT, sending you a quick hello"
	err = svc.SendVoiceNote(ctx, conversation.ID, tmpFile.Name(), ttsText)
	if err != nil {
		t.Fatalf("send voice note: %v", err)
	}

	msg, err := db.MessageGet(ctx, conn, db.MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "audio-transcript-1",
	})
	if err != nil {
		t.Fatalf("get outbound audio message: %v", err)
	}

	if msg.Body != ttsText {
		t.Fatalf("expected body %q, got %q", ttsText, msg.Body)
	}
	if !msg.MediaTranscriptText.Valid || msg.MediaTranscriptText.String != ttsText {
		t.Fatalf("expected media transcript %q, got %+v", ttsText, msg.MediaTranscriptText)
	}
	if !msg.MediaLocalPath.Valid || msg.MediaLocalPath.String == "" {
		t.Fatalf("expected non-empty media local path, got %+v", msg.MediaLocalPath)
	}
}

func TestMarkReadCapsAtReadUpTo(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	platform := &mockPlatform{platform: "whatsapp"}
	svc.RegisterPlatform(platform)

	externalConversationID := "conversation@s.whatsapp.net"
	externalSenderID := "alice@s.whatsapp.net"

	t1 := time.Date(2026, time.February, 27, 10, 7, 14, 0, time.UTC)
	t2 := time.Date(2026, time.February, 27, 10, 7, 15, 0, time.UTC)
	t3 := time.Date(2026, time.February, 27, 10, 7, 16, 0, time.UTC)

	sendInboundMessageForReadTest(t, ctx, svc, externalConversationID, externalSenderID, "in-1", "first", t1)
	sendInboundMessageForReadTest(t, ctx, svc, externalConversationID, externalSenderID, "in-2", "second", t2)
	sendInboundMessageForReadTest(t, ctx, svc, externalConversationID, externalSenderID, "in-3", "third", t3)

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: externalConversationID,
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	conversationID := conv.ID

	err = db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	err = db.SystemMessageInsert(ctx, conn, db.SystemMessageParams{
		ConversationID: conversationID,
		Kind:           "task_report",
		Body: map[string]any{
			"task_id": "deadbeef-cafe-fade-face-000000000001",
			"goal":    "ignored",
			"status":  "running",
			"content": "system note",
		},
		SentAt: t2,
	})
	if err != nil {
		t.Fatalf("insert system message: %v", err)
	}

	err = svc.MarkRead(ctx, conversationID, t2)
	if err != nil {
		t.Fatalf("mark read up to t2: %v", err)
	}

	if platform.markReadCalls != 1 {
		t.Fatalf("expected one platform mark-read call, got %d", platform.markReadCalls)
	}
	if len(platform.lastReadRefs) != 2 {
		t.Fatalf("expected two read refs, got %d", len(platform.lastReadRefs))
	}

	for _, ref := range platform.lastReadRefs {
		if ref.SentAt.After(t2) {
			t.Fatalf("expected no read refs after t2, got %v", ref.SentAt)
		}
	}

	conv, err = db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: conversationID})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if !conv.LastReadAt.Valid {
		t.Fatalf("expected last_read_at to be set")
	}
	if !conv.LastReadAt.Time.Equal(t2) {
		t.Fatalf("expected last_read_at to equal t2, got %v", conv.LastReadAt.Time)
	}

	err = svc.MarkRead(ctx, conversationID, t3)
	if err != nil {
		t.Fatalf("mark read up to t3: %v", err)
	}

	if platform.markReadCalls != 2 {
		t.Fatalf("expected two platform mark-read calls, got %d", platform.markReadCalls)
	}
	if len(platform.lastReadRefs) != 1 {
		t.Fatalf("expected one read ref on second call, got %d", len(platform.lastReadRefs))
	}
	if platform.lastReadRefs[0].ExternalMessageID != "in-3" {
		t.Fatalf("expected final read ref to be in-3, got %q", platform.lastReadRefs[0].ExternalMessageID)
	}

	err = svc.MarkRead(ctx, conversationID, time.Time{})
	if err != nil {
		t.Fatalf("mark read with zero readUpTo: %v", err)
	}
	if platform.markReadCalls != 2 {
		t.Fatalf("expected no additional mark-read call for zero readUpTo, got %d", platform.markReadCalls)
	}
}

func TestMarkReadPicksUpMessagesAfterOutbound(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)
	svc.replyDelay = func(string) time.Duration { return 0 }

	platform := &mockPlatform{
		platform: "whatsapp",
		outbound: OutboundMessage{
			ExternalMessageID: "nik-reply-1",
			ExternalSenderID:  "nik@s.whatsapp.net",
			Kind:              "text",
			Body:              "got it",
		},
	}
	svc.RegisterPlatform(platform)

	externalConversationID := "conversation@s.whatsapp.net"
	externalSenderID := "alice@s.whatsapp.net"

	t1 := time.Date(2026, time.February, 27, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, time.February, 27, 10, 0, 5, 0, time.UTC)
	t3 := time.Date(2026, time.February, 27, 10, 0, 6, 0, time.UTC)

	sendInboundMessageForReadTest(t, ctx, svc, externalConversationID, externalSenderID, "in-1", "hello", t1)

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: externalConversationID,
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	conversationID := conv.ID

	err = svc.MarkRead(ctx, conversationID, t1)
	if err != nil {
		t.Fatalf("mark read up to t1: %v", err)
	}

	if platform.markReadCalls != 1 {
		t.Fatalf("expected one mark-read call after first batch, got %d", platform.markReadCalls)
	}
	if len(platform.lastReadRefs) != 1 || platform.lastReadRefs[0].ExternalMessageID != "in-1" {
		t.Fatalf("expected mark-read for in-1, got %+v", platform.lastReadRefs)
	}

	platform.outbound.SentAt = t1.Add(3 * time.Second)
	err = svc.Reply(ctx, conversationID, "got it", nil)
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	conv, err = db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: conversationID})
	if err != nil {
		t.Fatalf("get conversation after reply: %v", err)
	}
	if conv.LastReadAt.Valid && conv.LastReadAt.Time.After(t1) {
		t.Fatalf("outbound reply should not advance last_read_at past t1, got %v", conv.LastReadAt.Time)
	}

	sendInboundMessageForReadTest(t, ctx, svc, externalConversationID, externalSenderID, "in-2", "also this", t2)
	sendInboundMessageForReadTest(t, ctx, svc, externalConversationID, externalSenderID, "in-3", "one more", t3)

	err = svc.MarkRead(ctx, conversationID, t3)
	if err != nil {
		t.Fatalf("mark read up to t3: %v", err)
	}

	if platform.markReadCalls != 2 {
		t.Fatalf("expected two total mark-read calls, got %d", platform.markReadCalls)
	}
	if len(platform.lastReadRefs) != 2 {
		t.Fatalf("expected two read refs for second batch, got %d", len(platform.lastReadRefs))
	}

	refIDs := map[string]bool{}
	for _, ref := range platform.lastReadRefs {
		refIDs[ref.ExternalMessageID] = true
	}
	if !refIDs["in-2"] || !refIDs["in-3"] {
		t.Fatalf("expected in-2 and in-3 in second mark-read batch, got %+v", platform.lastReadRefs)
	}
}

func TestFindMessage(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)
	ts := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)

	t.Run("exact match", func(t *testing.T) {
		convID := seedConversation(t, ctx, svc, "whatsapp", "conv-find-exact@s.whatsapp.net", "dm")
		seedMessage(t, ctx, svc, "conv-find-exact@s.whatsapp.net", "sender@s.whatsapp.net", "find-1", "unique hello", ts)
		seedMessage(t, ctx, svc, "conv-find-exact@s.whatsapp.net", "sender@s.whatsapp.net", "find-2", "unique world", ts.Add(time.Second))

		msg, findErr := svc.FindMessage(ctx, convID, "unique hello", "10:00:00")
		if findErr != nil {
			t.Fatalf("find message: %v", findErr)
		}
		if msg.Body != "unique hello" {
			t.Fatalf("expected body 'unique hello', got %q", msg.Body)
		}
	})

	t.Run("no match", func(t *testing.T) {
		convID := seedConversation(t, ctx, svc, "whatsapp", "conv-find-none@s.whatsapp.net", "dm")
		seedMessage(t, ctx, svc, "conv-find-none@s.whatsapp.net", "sender@s.whatsapp.net", "nm-1", "hello", ts)

		_, findErr := svc.FindMessage(ctx, convID, "nonexistent", "10:00:00")
		if findErr == nil {
			t.Fatalf("expected error for no match")
		}
		if !strings.Contains(findErr.Error(), "no message matching") {
			t.Fatalf("expected 'no message matching' error, got %v", findErr)
		}
	})

	t.Run("wrong time no match", func(t *testing.T) {
		convID := seedConversation(t, ctx, svc, "whatsapp", "conv-find-wrongtime@s.whatsapp.net", "dm")
		seedMessage(t, ctx, svc, "conv-find-wrongtime@s.whatsapp.net", "sender@s.whatsapp.net", "wt-1", "hello", ts)

		_, findErr := svc.FindMessage(ctx, convID, "hello", "11:00:00")
		if findErr == nil {
			t.Fatalf("expected error for wrong time")
		}
	})

	t.Run("same text disambiguates by time", func(t *testing.T) {
		t2 := ts.Add(5 * time.Second)
		convID := seedConversation(t, ctx, svc, "whatsapp", "conv-find-disamb@s.whatsapp.net", "dm")
		seedMessage(t, ctx, svc, "conv-find-disamb@s.whatsapp.net", "sender@s.whatsapp.net", "dis-1", "ok", ts)
		seedMessage(t, ctx, svc, "conv-find-disamb@s.whatsapp.net", "sender@s.whatsapp.net", "dis-2", "ok", t2)

		msg, findErr := svc.FindMessage(ctx, convID, "ok", "10:00:00")
		if findErr != nil {
			t.Fatalf("find first ok: %v", findErr)
		}
		if msg.ExternalMessageID != "dis-1" {
			t.Fatalf("expected dis-1, got %q", msg.ExternalMessageID)
		}

		msg, findErr = svc.FindMessage(ctx, convID, "ok", "10:00:05")
		if findErr != nil {
			t.Fatalf("find second ok: %v", findErr)
		}
		if msg.ExternalMessageID != "dis-2" {
			t.Fatalf("expected dis-2, got %q", msg.ExternalMessageID)
		}
	})

	t.Run("collision picks most recent", func(t *testing.T) {
		convID := seedConversation(t, ctx, svc, "whatsapp", "conv-find-collision@s.whatsapp.net", "dm")
		seedMessage(t, ctx, svc, "conv-find-collision@s.whatsapp.net", "sender@s.whatsapp.net", "col-1", "ok", ts)
		seedMessage(t, ctx, svc, "conv-find-collision@s.whatsapp.net", "sender@s.whatsapp.net", "col-2", "ok", ts)

		msg, findErr := svc.FindMessage(ctx, convID, "ok", "10:00:00")
		if findErr != nil {
			t.Fatalf("find collision: %v", findErr)
		}
		if msg.Body != "ok" {
			t.Fatalf("expected body 'ok', got %q", msg.Body)
		}
	})

	t.Run("substring does not match", func(t *testing.T) {
		convID := seedConversation(t, ctx, svc, "whatsapp", "conv-find-substr@s.whatsapp.net", "dm")
		seedMessage(t, ctx, svc, "conv-find-substr@s.whatsapp.net", "sender@s.whatsapp.net", "sub-1", "hello world", ts)

		_, findErr := svc.FindMessage(ctx, convID, "hello", "10:00:00")
		if findErr == nil {
			t.Fatalf("expected error: substring should not match")
		}
	})
}

func TestConversationHeaderUnifiedDM(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	seedConversation(t, ctx, svc, "whatsapp", "conv-dm@s.whatsapp.net", "dm")

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-dm@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	session := svc.ConversationHeader(ctx, conv)
	out := strings.Join(session.Lines, "\n")

	if !strings.Contains(out, "id: "+conv.ID) {
		t.Fatalf("expected id line in header, got %q", out)
	}
	if !strings.Contains(out, "title:") {
		t.Fatalf("expected title in header, got %q", out)
	}
	if !strings.Contains(out, "platform: whatsapp") {
		t.Fatalf("expected platform in header, got %q", out)
	}
	if !strings.Contains(out, "type: dm") {
		t.Fatalf("expected type in header, got %q", out)
	}
	if strings.Contains(out, "Announce mode") {
		t.Fatalf("should not contain Announce mode, got %q", out)
	}
	if strings.Contains(out, "Contact profile") {
		t.Fatalf("should not contain Contact profile block, got %q", out)
	}
}

func TestParticipantGaps(t *testing.T) {
	tests := []struct {
		name string
		p    db.ConversationParticipant
		want string
	}{
		{
			name: "all fields empty",
			p:    db.ConversationParticipant{},
			want: "[needs: name, timezone, location, one_liner]",
		},
		{
			name: "all fields populated",
			p: db.ConversationParticipant{
				ContactName: validString("Alice"),
				Timezone:    validString("America/New_York"),
				Location:    validString("New York"),
				OneLiner:    validString("Friend from college"),
			},
			want: "",
		},
		{
			name: "only name set",
			p: db.ConversationParticipant{
				ContactName: validString("Bob"),
			},
			want: "[needs: timezone, location, one_liner]",
		},
		{
			name: "whitespace-only fields treated as empty",
			p: db.ConversationParticipant{
				ContactName: validString("  "),
				Timezone:    validString("  "),
			},
			want: "[needs: name, timezone, location, one_liner]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := participantGaps(tt.p)
			if got != tt.want {
				t.Errorf("participantGaps() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConversationHeaderGaps(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	now := time.Now()
	err = svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "peer@s.whatsapp.net",
		ExternalMessageID:      "msg-from-me",
		ExternalSenderID:       "nik@s.whatsapp.net",
		Kind:                   "text",
		Body:                   "hello",
		SentAt:                 now,
		IsFromMe:               true,
	})
	if err != nil {
		t.Fatalf("receive from-me message: %v", err)
	}

	err = svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "peer@s.whatsapp.net",
		ExternalMessageID:      "msg-from-peer",
		ExternalSenderID:       "peer@s.whatsapp.net",
		Kind:                   "text",
		Body:                   "hi",
		SentAt:                 now.Add(time.Second),
		IsFromMe:               false,
	})
	if err != nil {
		t.Fatalf("receive peer message: %v", err)
	}

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "peer@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	session := svc.ConversationHeader(ctx, conv)
	lines := session.Lines

	nikGaps := false
	peerGaps := false
	for i, line := range lines {
		if !strings.HasPrefix(line, "- ") {
			continue
		}

		isNik := strings.Contains(line, contacts.NikContactID)
		hasGaps := false
		for j := i + 1; j < len(lines) && strings.HasPrefix(lines[j], "  "); j++ {
			if strings.Contains(lines[j], "[needs:") {
				hasGaps = true
			}
		}

		if isNik && hasGaps {
			nikGaps = true
		}
		if !isNik && hasGaps {
			peerGaps = true
		}
	}

	if nikGaps {
		t.Errorf("nik's participant should not show gaps, got:\n%s", strings.Join(lines, "\n"))
	}
	if !peerGaps {
		t.Errorf("peer participant should show gaps, got:\n%s", strings.Join(lines, "\n"))
	}
}

func TestReceiveMessageDMReusesConversationOnJIDChange(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = db.ContactUpsert(ctx, conn, db.ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "bob@lid",
		Name:          "Bob",
		Phone:         "bob",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	now := time.Now()
	err = svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "bob@lid",
		ExternalMessageID:      "msg-lid-1",
		ExternalSenderID:       "bob@lid",
		Kind:                   "text",
		Body:                   "hello via lid",
		SentAt:                 now,
	})
	if err != nil {
		t.Fatalf("receive first message: %v", err)
	}

	conv1, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "bob@lid",
	})
	if err != nil {
		t.Fatalf("get conversation by lid: %v", err)
	}

	err = svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: "bob@s.whatsapp.net",
		ExternalMessageID:      "msg-phone-1",
		ExternalSenderID:       "bob@s.whatsapp.net",
		ExternalSenderIDs:      []string{"bob@s.whatsapp.net", "bob@lid"},
		Kind:                   "text",
		Body:                   "hello via phone jid",
		SentAt:                 now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("receive second message: %v", err)
	}

	msg, err := db.MessageGet(ctx, conn, db.MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "msg-phone-1",
	})
	if err != nil {
		t.Fatalf("get second message: %v", err)
	}

	if msg.ConversationID != conv1.ID {
		t.Fatalf("expected same conversation %s, got %s", conv1.ID, msg.ConversationID)
	}

	conv1Updated, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: conv1.ID})
	if err != nil {
		t.Fatalf("get updated conversation: %v", err)
	}
	if conv1Updated.ExternalConversationID != "bob@s.whatsapp.net" {
		t.Fatalf("expected external_conversation_id updated to bob@s.whatsapp.net, got %s", conv1Updated.ExternalConversationID)
	}
}

func TestReplyRejectsBannedWords(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	cfg := &config.Config{BannedWords: []string{"forbidden", "BLOCKED"}}
	svc := NewService(cfg, conn, contactsSvc)
	svc.replyDelay = func(string) time.Duration { return 0 }

	now := time.Now()
	err = db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-banned@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-banned@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	platform := &mockPlatform{
		platform: "whatsapp",
		outbound: OutboundMessage{
			ExternalMessageID: "out-ban",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "text",
			Body:              "clean",
		},
	}
	svc.RegisterPlatform(platform)

	err = svc.Reply(ctx, conv.ID, "this is forbidden content", nil)
	if err == nil {
		t.Fatalf("expected error for banned word")
	}
	if !strings.Contains(err.Error(), "banned word") {
		t.Fatalf("expected banned word error, got %v", err)
	}
	if platform.replyCalls != 0 {
		t.Fatalf("expected no platform reply calls, got %d", platform.replyCalls)
	}

	// case-insensitive: "blocked" in config as "BLOCKED", message has "Blocked"
	err = svc.Reply(ctx, conv.ID, "this is Blocked too", nil)
	if err == nil {
		t.Fatalf("expected error for case-insensitive banned word")
	}

	// clean message should go through
	err = svc.Reply(ctx, conv.ID, "clean message", nil)
	if err != nil {
		t.Fatalf("clean reply: %v", err)
	}
	if platform.replyCalls != 1 {
		t.Fatalf("expected one platform reply call for clean message, got %d", platform.replyCalls)
	}
}

func TestSendFileRejectsBannedWordsInCaption(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	cfg := &config.Config{BannedWords: []string{"nope"}}
	svc := NewService(cfg, conn, contactsSvc)
	svc.replyDelay = func(string) time.Duration { return 0 }

	now := time.Now()
	err = db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-img-ban@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-img-ban@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	platform := &mockPlatform{
		platform: "whatsapp",
		fileOutbound: OutboundMessage{
			ExternalMessageID: "img-ban",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "image",
			MimeType:          "image/jpeg",
		},
	}
	svc.RegisterPlatform(platform)

	tmpFile, err := os.CreateTemp("", "test-ban-img-*.jpg")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.Write([]byte("fake"))
	tmpFile.Close()

	err = svc.SendFile(ctx, conv.ID, tmpFile.Name(), "nope bad caption")
	if err == nil {
		t.Fatalf("expected error for banned word in caption")
	}
	if !strings.Contains(err.Error(), "banned word") {
		t.Fatalf("expected banned word error, got %v", err)
	}
	if platform.sendFileCalls != 0 {
		t.Fatalf("expected no platform send file calls, got %d", platform.sendFileCalls)
	}
}

func TestResolveConversation(t *testing.T) {
	t.Run("creates dm conversation", func(t *testing.T) {
		ctx := context.Background()

		conn, err := db.OpenInMemory()
		if err != nil {
			t.Fatalf("open in-memory db: %v", err)
		}
		defer conn.Close()

		_, err = db.ContactUpsert(ctx, conn, db.ContactUpsertParams{
			Platform:      "whatsapp",
			ExternalID:    "55555@s.whatsapp.net",
			Name:          "Charlie",
			Phone:         "55555",
			LastMessageAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("seed contact: %v", err)
		}

		contact, err := db.ContactGet(ctx, conn, "55555@s.whatsapp.net")
		if err != nil {
			t.Fatalf("get contact: %v", err)
		}

		contactsSvc := contacts.NewService(conn)
		svc := NewService(&config.Config{}, conn, contactsSvc)

		convID, err := svc.ResolveConversation(ctx, contact.ID)
		if err != nil {
			t.Fatalf("resolve conversation: %v", err)
		}
		if convID == "" {
			t.Fatalf("expected non-empty conversation id")
		}

		conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: convID})
		if err != nil {
			t.Fatalf("get conversation: %v", err)
		}
		if conv.Platform != "whatsapp" {
			t.Fatalf("expected platform whatsapp, got %q", conv.Platform)
		}
		if conv.ExternalConversationID != "55555@s.whatsapp.net" {
			t.Fatalf("expected external_conversation_id 55555@s.whatsapp.net, got %q", conv.ExternalConversationID)
		}
		if conv.Kind != "dm" {
			t.Fatalf("expected kind dm, got %q", conv.Kind)
		}

		participants, err := db.ConversationParticipantList(ctx, conn, convID)
		if err != nil {
			t.Fatalf("get participants: %v", err)
		}
		if len(participants) != 1 {
			t.Fatalf("expected 1 participant, got %d", len(participants))
		}
		if participants[0].ContactID != contact.ID {
			t.Fatalf("expected participant %s, got %s", contact.ID, participants[0].ContactID)
		}

		convID2, err := svc.ResolveConversation(ctx, contact.ID)
		if err != nil {
			t.Fatalf("idempotent resolve: %v", err)
		}
		if convID != convID2 {
			t.Fatalf("expected same conversation id, got %q and %q", convID, convID2)
		}
	})

	t.Run("no whatsapp id", func(t *testing.T) {
		ctx := context.Background()

		conn, err := db.OpenInMemory()
		if err != nil {
			t.Fatalf("open in-memory db: %v", err)
		}
		defer conn.Close()

		_, err = conn.ExecContext(ctx,
			`INSERT INTO contact (id, name) VALUES ('no-wa-contact', 'NoWA')`,
		)
		if err != nil {
			t.Fatalf("seed contact: %v", err)
		}

		contactsSvc := contacts.NewService(conn)
		svc := NewService(&config.Config{}, conn, contactsSvc)

		_, err = svc.ResolveConversation(ctx, "no-wa-contact")
		if err == nil {
			t.Fatalf("expected error for contact with no whatsapp id")
		}
		if !strings.Contains(err.Error(), "no whatsapp id") {
			t.Fatalf("expected 'no whatsapp id' error, got %v", err)
		}
	})
}

func seedConversation(t *testing.T, ctx context.Context, svc *Service, platform, externalConvID, kind string) string {
	t.Helper()

	now := time.Now()
	err := db.ConversationUpsert(ctx, svc.db, db.ConversationUpsertParams{
		Platform:               platform,
		ExternalConversationID: externalConvID,
		Kind:                   kind,
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := db.ConversationGet(ctx, svc.db, db.ConversationGetParams{
		Platform:               platform,
		ExternalConversationID: externalConvID,
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	return conv.ID
}

func seedMessage(t *testing.T, ctx context.Context, svc *Service, externalConvID, externalSenderID, externalMsgID, body string, sentAt time.Time) {
	t.Helper()

	err := svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: externalConvID,
		ExternalMessageID:      externalMsgID,
		ExternalSenderID:       externalSenderID,
		Kind:                   "text",
		Body:                   body,
		SentAt:                 sentAt,
		IsFromMe:               false,
		IsGroup:                false,
	})
	if err != nil {
		t.Fatalf("seed message %s: %v", externalMsgID, err)
	}
}

func sendInboundMessageForReadTest(
	t *testing.T,
	ctx context.Context,
	svc *Service,
	externalConversationID string,
	externalSenderID string,
	externalMessageID string,
	body string,
	sentAt time.Time,
) {
	t.Helper()

	err := svc.ReceiveMessage(ctx, InboundMessage{
		Platform:               "whatsapp",
		ExternalConversationID: externalConversationID,
		ExternalMessageID:      externalMessageID,
		ExternalSenderID:       externalSenderID,
		Kind:                   "text",
		Body:                   body,
		SentAt:                 sentAt,
		IsFromMe:               false,
		IsGroup:                false,
	})
	if err != nil {
		t.Fatalf("receive inbound message %s: %v", externalMessageID, err)
	}
}

func TestTouchPresenceSetsAvailableOnFirstSend(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	now := time.Now()
	platform := &mockPlatform{
		platform: "whatsapp",
		outbound: OutboundMessage{
			ExternalMessageID: "reply-1",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "text",
			Body:              "hi",
		},
	}
	svc.RegisterPlatform(platform)
	svc.replyDelay = func(_ string) time.Duration { return 0 }

	convID := seedConversation(t, ctx, svc, "whatsapp", "chat@s.whatsapp.net", "dm")

	err = svc.Reply(ctx, convID, "hi", nil)
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	if platform.setPresenceCalls != 1 {
		t.Fatalf("expected 1 SetPresence call, got %d", platform.setPresenceCalls)
	}
	if !platform.lastPresenceOnline {
		t.Fatalf("expected presence to be online")
	}
}

func TestTouchPresenceDebounces(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	now := time.Now()
	platform := &mockPlatform{
		platform: "whatsapp",
		outbound: OutboundMessage{
			ExternalSenderID: "nik@s.whatsapp.net",
			SentAt:           now,
			Kind:             "text",
			Body:             "hi",
		},
	}
	svc.RegisterPlatform(platform)
	svc.replyDelay = func(_ string) time.Duration { return 0 }

	convID := seedConversation(t, ctx, svc, "whatsapp", "chat@s.whatsapp.net", "dm")

	for i := 0; i < 3; i++ {
		platform.outbound.ExternalMessageID = fmt.Sprintf("reply-%d", i+1)
		err = svc.Reply(ctx, convID, "hi", nil)
		if err != nil {
			t.Fatalf("reply %d: %v", i+1, err)
		}
	}

	if platform.setPresenceCalls != 1 {
		t.Fatalf("expected 1 SetPresence call (debounced), got %d", platform.setPresenceCalls)
	}

	svc.StopPresence()
	if platform.setPresenceCalls != 2 {
		t.Fatalf("expected 2 SetPresence calls after StopPresence, got %d", platform.setPresenceCalls)
	}
	if platform.lastPresenceOnline {
		t.Fatalf("expected presence to be offline after StopPresence")
	}
}

func validString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}
