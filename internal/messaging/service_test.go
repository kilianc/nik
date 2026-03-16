package messaging

import (
	"context"
	"errors"
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

	msg, err := db.GetMessage(ctx, conn, db.GetMessageParams{
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

	_, err = db.UpsertContact(ctx, conn, db.UpsertContactParams{
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

	msg, err := db.GetMessage(ctx, conn, db.GetMessageParams{
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
	platform         string
	startTypingCalls int
	stopTypingCalls  int
	replyCalls       int
	sendImageCalls   int
	sendAudioCalls   int
	markReadCalls    int
	lastReadRefs     []InboundMessage
	outbound         OutboundMessage
	imageOutbound    OutboundMessage
	audioOutbound    OutboundMessage
}

func (m *mockPlatform) Platform() string { return m.platform }
func (m *mockPlatform) Start(_ context.Context, _ MessageReceiver) error {
	return nil
}
func (m *mockPlatform) Stop(_ context.Context) error { return nil }
func (m *mockPlatform) Reply(_ context.Context, _ string, _ string) (OutboundMessage, error) {
	m.replyCalls++
	return m.outbound, nil
}
func (m *mockPlatform) SendImage(_ context.Context, _ string, _ string, _ string) (OutboundMessage, error) {
	m.sendImageCalls++
	return m.imageOutbound, nil
}
func (m *mockPlatform) SendAudio(_ context.Context, _ string, _ string, _ bool) (OutboundMessage, error) {
	m.sendAudioCalls++
	return m.audioOutbound, nil
}
func (m *mockPlatform) React(_ context.Context, _, _, _, _ string) (OutboundMessage, error) {
	return OutboundMessage{}, nil
}
func (m *mockPlatform) SetPresence(_ context.Context, _ bool) error { return nil }
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
	err = db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.GetConversation(ctx, conn, db.GetConversationParams{
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

	err = svc.Reply(ctx, conversationID, "hello")
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	msg, err := db.GetMessage(ctx, conn, db.GetMessageParams{
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

func TestSendImagePersistsOutbound(t *testing.T) {
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
	err = db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.GetConversation(ctx, conn, db.GetConversationParams{
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
		imageOutbound: OutboundMessage{
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

	err = svc.SendImage(ctx, conversation.ID, tmpFile.Name(), "check this out")
	if err != nil {
		t.Fatalf("send image: %v", err)
	}

	msg, err := db.GetMessage(ctx, conn, db.GetMessageParams{
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
	if platform.sendImageCalls != 1 {
		t.Fatalf("expected one send image call, got %d", platform.sendImageCalls)
	}
	if platform.startTypingCalls != 1 || platform.stopTypingCalls != 1 {
		t.Fatalf("expected start/stop typing once each, got start=%d stop=%d", platform.startTypingCalls, platform.stopTypingCalls)
	}
}

func TestSendAudioPersistsOutbound(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	now := time.Now()
	err = db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.GetConversation(ctx, conn, db.GetConversationParams{
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
		audioOutbound: OutboundMessage{
			ExternalMessageID: "audio-1",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "audio",
			MimeType:          "audio/ogg; codecs=opus",
			LocalPath:         "abc123.ogg",
		},
	}
	svc.RegisterPlatform(platform)

	err = svc.SendAudio(ctx, conversation.ID, tmpFile.Name(), true, "")
	if err != nil {
		t.Fatalf("send audio: %v", err)
	}

	msg, err := db.GetMessage(ctx, conn, db.GetMessageParams{
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
	if platform.sendAudioCalls != 1 {
		t.Fatalf("expected one send audio call, got %d", platform.sendAudioCalls)
	}
}

func TestSendAudioStoresTranscript(t *testing.T) {
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
	err = db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conversation@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := db.GetConversation(ctx, conn, db.GetConversationParams{
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
		audioOutbound: OutboundMessage{
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
	err = svc.SendAudio(ctx, conversation.ID, tmpFile.Name(), true, ttsText)
	if err != nil {
		t.Fatalf("send audio: %v", err)
	}

	msg, err := db.GetMessage(ctx, conn, db.GetMessageParams{
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

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: externalConversationID,
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	conversationID := conv.ID

	err = db.EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	err = db.InsertSystemMessage(ctx, conn, db.SystemMessageParams{
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

	conv, err = db.GetConversation(ctx, conn, db.GetConversationParams{ID: conversationID})
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
}

func TestMarkReadSkipsZeroReadUpTo(t *testing.T) {
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

	sendInboundMessageForReadTest(t, ctx, svc, externalConversationID, externalSenderID, "in-1", "first", t1)

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: externalConversationID,
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	conversationID := conv.ID

	err = svc.MarkRead(ctx, conversationID, time.Time{})
	if err != nil {
		t.Fatalf("mark read with zero readUpTo: %v", err)
	}

	if platform.markReadCalls != 0 {
		t.Fatalf("expected no platform mark-read calls, got %d", platform.markReadCalls)
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

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
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
	err = svc.Reply(ctx, conversationID, "got it")
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	conv, err = db.GetConversation(ctx, conn, db.GetConversationParams{ID: conversationID})
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

func TestFindMessageExactMatch(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	ts := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)
	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-find@s.whatsapp.net", "dm")
	seedMessage(t, ctx, svc, "conv-find@s.whatsapp.net", "sender@s.whatsapp.net", "find-1", "unique hello", ts)
	seedMessage(t, ctx, svc, "conv-find@s.whatsapp.net", "sender@s.whatsapp.net", "find-2", "unique world", ts.Add(time.Second))

	msg, err := svc.FindMessage(ctx, convID, "unique hello", "10:00:00")
	if err != nil {
		t.Fatalf("find message: %v", err)
	}
	if msg.Body != "unique hello" {
		t.Fatalf("expected body 'unique hello', got %q", msg.Body)
	}
}

func TestFindMessageNoMatch(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	ts := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)
	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-nomatch@s.whatsapp.net", "dm")
	seedMessage(t, ctx, svc, "conv-nomatch@s.whatsapp.net", "sender@s.whatsapp.net", "nm-1", "hello", ts)

	_, err = svc.FindMessage(ctx, convID, "nonexistent", "10:00:00")
	if err == nil {
		t.Fatalf("expected error for no match")
	}
	if !strings.Contains(err.Error(), "no message matching") {
		t.Fatalf("expected 'no message matching' error, got %v", err)
	}
}

func TestFindMessageWrongTimeNoMatch(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	ts := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)
	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-wrongtime@s.whatsapp.net", "dm")
	seedMessage(t, ctx, svc, "conv-wrongtime@s.whatsapp.net", "sender@s.whatsapp.net", "wt-1", "hello", ts)

	_, err = svc.FindMessage(ctx, convID, "hello", "11:00:00")
	if err == nil {
		t.Fatalf("expected error for wrong time")
	}
}

func TestFindMessageSameTextDifferentTimesDisambiguates(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	t1 := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, time.February, 28, 10, 0, 5, 0, time.UTC)
	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-disamb@s.whatsapp.net", "dm")
	seedMessage(t, ctx, svc, "conv-disamb@s.whatsapp.net", "sender@s.whatsapp.net", "dis-1", "ok", t1)
	seedMessage(t, ctx, svc, "conv-disamb@s.whatsapp.net", "sender@s.whatsapp.net", "dis-2", "ok", t2)

	msg, err := svc.FindMessage(ctx, convID, "ok", "10:00:00")
	if err != nil {
		t.Fatalf("find first ok: %v", err)
	}
	if msg.ExternalMessageID != "dis-1" {
		t.Fatalf("expected dis-1, got %q", msg.ExternalMessageID)
	}

	msg, err = svc.FindMessage(ctx, convID, "ok", "10:00:05")
	if err != nil {
		t.Fatalf("find second ok: %v", err)
	}
	if msg.ExternalMessageID != "dis-2" {
		t.Fatalf("expected dis-2, got %q", msg.ExternalMessageID)
	}
}

func TestFindMessageCollisionPicksMostRecent(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	ts := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)
	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-collision@s.whatsapp.net", "dm")
	seedMessage(t, ctx, svc, "conv-collision@s.whatsapp.net", "sender@s.whatsapp.net", "col-1", "ok", ts)
	seedMessage(t, ctx, svc, "conv-collision@s.whatsapp.net", "sender@s.whatsapp.net", "col-2", "ok", ts)

	msg, err := svc.FindMessage(ctx, convID, "ok", "10:00:00")
	if err != nil {
		t.Fatalf("find collision: %v", err)
	}
	if msg.Body != "ok" {
		t.Fatalf("expected body 'ok', got %q", msg.Body)
	}
}

func TestFindMessageSubstringDoesNotMatch(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	ts := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)
	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-substr@s.whatsapp.net", "dm")
	seedMessage(t, ctx, svc, "conv-substr@s.whatsapp.net", "sender@s.whatsapp.net", "sub-1", "hello world", ts)

	_, err = svc.FindMessage(ctx, convID, "hello", "10:00:00")
	if err == nil {
		t.Fatalf("expected error: substring should not match")
	}
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

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-dm@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	session := svc.ConversationHeader(ctx, conv)
	out := strings.Join(session.Lines, "\n")

	if !strings.Contains(out, "Conversation: "+conv.ID) {
		t.Fatalf("expected Conversation ID in header, got %q", out)
	}
	if !strings.Contains(out, "Title:") {
		t.Fatalf("expected Title in header, got %q", out)
	}
	if !strings.Contains(out, "Platform: whatsapp") {
		t.Fatalf("expected Platform in header, got %q", out)
	}
	if !strings.Contains(out, "Type: dm") {
		t.Fatalf("expected Type in header, got %q", out)
	}
	if strings.Contains(out, "Announce mode") {
		t.Fatalf("should not contain Announce mode, got %q", out)
	}
	if strings.Contains(out, "Contact profile") {
		t.Fatalf("should not contain Contact profile block, got %q", out)
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
	err = db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-banned@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
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

	err = svc.Reply(ctx, conv.ID, "this is forbidden content")
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
	err = svc.Reply(ctx, conv.ID, "this is Blocked too")
	if err == nil {
		t.Fatalf("expected error for case-insensitive banned word")
	}

	// clean message should go through
	err = svc.Reply(ctx, conv.ID, "clean message")
	if err != nil {
		t.Fatalf("clean reply: %v", err)
	}
	if platform.replyCalls != 1 {
		t.Fatalf("expected one platform reply call for clean message, got %d", platform.replyCalls)
	}
}

func TestSendImageRejectsBannedWordsInCaption(t *testing.T) {
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
	err = db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-img-ban@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-img-ban@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	platform := &mockPlatform{
		platform: "whatsapp",
		imageOutbound: OutboundMessage{
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

	err = svc.SendImage(ctx, conv.ID, tmpFile.Name(), "nope bad caption")
	if err == nil {
		t.Fatalf("expected error for banned word in caption")
	}
	if !strings.Contains(err.Error(), "banned word") {
		t.Fatalf("expected banned word error, got %v", err)
	}
	if platform.sendImageCalls != 0 {
		t.Fatalf("expected no platform send image calls, got %d", platform.sendImageCalls)
	}
}

func TestReplyNoBannedWordsConfigured(t *testing.T) {
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
	err = db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-noban@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "conv-noban@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	platform := &mockPlatform{
		platform: "whatsapp",
		outbound: OutboundMessage{
			ExternalMessageID: "out-noban",
			ExternalSenderID:  "nik@s.whatsapp.net",
			SentAt:            now,
			Kind:              "text",
			Body:              "anything goes",
		},
	}
	svc.RegisterPlatform(platform)

	err = svc.Reply(ctx, conv.ID, "anything goes")
	if err != nil {
		t.Fatalf("reply with no banned words configured: %v", err)
	}
	if platform.replyCalls != 1 {
		t.Fatalf("expected one reply call, got %d", platform.replyCalls)
	}
}

func TestResolveConversationCreatesNewConversation(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = db.UpsertContact(ctx, conn, db.UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "55555@s.whatsapp.net",
		Name:          "Charlie",
		Phone:         "55555",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	contact, err := db.GetContact(ctx, conn, "55555@s.whatsapp.net")
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

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{ID: convID})
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

	participants, err := db.GetConversationParticipants(ctx, conn, convID)
	if err != nil {
		t.Fatalf("get participants: %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("expected 1 participant, got %d", len(participants))
	}
	if participants[0].ContactID != contact.ID {
		t.Fatalf("expected participant %s, got %s", contact.ID, participants[0].ContactID)
	}
}

func TestResolveConversationIdempotent(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = db.UpsertContact(ctx, conn, db.UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "66666@s.whatsapp.net",
		Name:          "Dana",
		Phone:         "66666",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	contact, err := db.GetContact(ctx, conn, "66666@s.whatsapp.net")
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	convID1, err := svc.ResolveConversation(ctx, contact.ID)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}

	convID2, err := svc.ResolveConversation(ctx, contact.ID)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}

	if convID1 != convID2 {
		t.Fatalf("expected same conversation id, got %q and %q", convID1, convID2)
	}
}

func TestResolveConversationNoWhatsappID(t *testing.T) {
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
}

func seedConversation(t *testing.T, ctx context.Context, svc *Service, platform, externalConvID, kind string) string {
	t.Helper()

	now := time.Now()
	err := db.UpsertConversation(ctx, svc.db, db.UpsertConversationParams{
		Platform:               platform,
		ExternalConversationID: externalConvID,
		Kind:                   kind,
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := db.GetConversation(ctx, svc.db, db.GetConversationParams{
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
