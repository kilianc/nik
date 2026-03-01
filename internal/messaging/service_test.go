package messaging

import (
	"context"
	"errors"
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

func TestBuildConversationInputRendersResolvedContactName(t *testing.T) {
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

	conv, msgs, err := svc.ConversationWithMessages(ctx, msg.ConversationID, 10)
	if err != nil {
		t.Fatalf("conversation with messages: %v", err)
	}

	labels := svc.SenderLabels(ctx, msgs)
	lines := BuildConversationInput(conv, msgs, labels, svc.SessionContext(ctx, conv))
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "Alice: hello") {
		t.Fatalf("expected resolved contact name in output, got %q", out)
	}
}

type mockPlatform struct {
	platform         string
	startTypingCalls int
	stopTypingCalls  int
	replyCalls       int
	markReadCalls    int
	lastReadRefs     []InboundMessage
	outbound         OutboundMessage
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
func (m *mockPlatform) React(_ context.Context, _, _, _, _ string) error { return nil }
func (m *mockPlatform) SetPresence(_ context.Context, _ bool) error      { return nil }
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

	conversationID, err := svc.ConversationIDFromExternal(ctx, "whatsapp", externalConversationID)
	if err != nil {
		t.Fatalf("conversation id from external: %v", err)
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

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{ID: conversationID})
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

	conversationID, err := svc.ConversationIDFromExternal(ctx, "whatsapp", externalConversationID)
	if err != nil {
		t.Fatalf("conversation id from external: %v", err)
	}

	err = svc.MarkRead(ctx, conversationID, time.Time{})
	if err != nil {
		t.Fatalf("mark read with zero readUpTo: %v", err)
	}

	if platform.markReadCalls != 0 {
		t.Fatalf("expected no platform mark-read calls, got %d", platform.markReadCalls)
	}
}

func TestFindMessageUniqueMatch(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-find@s.whatsapp.net", "dm")
	seedMessage(t, ctx, svc, "conv-find@s.whatsapp.net", "sender@s.whatsapp.net", "find-1", "unique hello", time.Now())
	seedMessage(t, ctx, svc, "conv-find@s.whatsapp.net", "sender@s.whatsapp.net", "find-2", "unique world", time.Now())

	msg, err := svc.FindMessage(ctx, convID, "unique hello")
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

	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-nomatch@s.whatsapp.net", "dm")
	seedMessage(t, ctx, svc, "conv-nomatch@s.whatsapp.net", "sender@s.whatsapp.net", "nm-1", "hello", time.Now())

	_, err = svc.FindMessage(ctx, convID, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for no match")
	}
	if !strings.Contains(err.Error(), "no message matching") {
		t.Fatalf("expected 'no message matching' error, got %v", err)
	}
}

func TestFindMessageAmbiguousReturnsError(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = db.UpsertContact(ctx, conn, db.UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "alice@s.whatsapp.net",
		Name:          "Alice",
		Phone:         "11111",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact alice: %v", err)
	}
	_, err = db.UpsertContact(ctx, conn, db.UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "bob@s.whatsapp.net",
		Name:          "Bob",
		Phone:         "22222",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact bob: %v", err)
	}

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-ambig@s.whatsapp.net", "group")
	t1 := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, time.February, 28, 10, 0, 5, 0, time.UTC)
	seedMessage(t, ctx, svc, "conv-ambig@s.whatsapp.net", "alice@s.whatsapp.net", "amb-1", "ok", t1)
	seedMessage(t, ctx, svc, "conv-ambig@s.whatsapp.net", "bob@s.whatsapp.net", "amb-2", "ok", t2)

	_, err = svc.FindMessage(ctx, convID, "ok")
	if err == nil {
		t.Fatalf("expected ambiguous match error")
	}
	if !strings.Contains(err.Error(), "messages match") {
		t.Fatalf("expected 'messages match' error, got %v", err)
	}

	// disambiguate with sender
	msg, err := svc.FindMessage(ctx, convID, "Alice: ok")
	if err != nil {
		t.Fatalf("find message with sender: %v", err)
	}
	if msg.Body != "ok" {
		t.Fatalf("expected body 'ok', got %q", msg.Body)
	}
}

func TestFindMessageIdenticalPicksMostRecent(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contactsSvc := contacts.NewService(conn)
	svc := NewService(&config.Config{}, conn, contactsSvc)

	convID := seedConversation(t, ctx, svc, "whatsapp", "conv-ident@s.whatsapp.net", "dm")
	ts := time.Date(2026, time.February, 28, 10, 0, 0, 0, time.UTC)
	seedMessage(t, ctx, svc, "conv-ident@s.whatsapp.net", "sender@s.whatsapp.net", "id-1", "ok", ts)
	seedMessage(t, ctx, svc, "conv-ident@s.whatsapp.net", "sender@s.whatsapp.net", "id-2", "ok", ts)

	msg, err := svc.FindMessage(ctx, convID, "ok")
	if err != nil {
		t.Fatalf("find identical messages: %v", err)
	}

	// should pick the most recent (last inserted)
	if msg.Body != "ok" {
		t.Fatalf("expected body 'ok', got %q", msg.Body)
	}
}

func TestSessionContextUnifiedDM(t *testing.T) {
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

	session := svc.SessionContext(ctx, conv)
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
