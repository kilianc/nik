package timeline

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
)

func setupTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()

	_, err = db.UpsertContact(ctx, conn, db.UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "sender@s.whatsapp.net",
		Name:          "Sender",
		Phone:         "11111",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	contact, err := db.GetContact(ctx, conn, "sender@s.whatsapp.net")
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	now := time.Now()
	err = db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "ext-conv@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "ext-conv@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	_ = contact
	return conn, conv.ID
}

func insertMsg(t *testing.T, conn *sql.DB, convID string, id string, extMsgID string, kind string, body string, sentAt time.Time) {
	t.Helper()

	contact, err := db.GetContact(context.Background(), conn, "sender@s.whatsapp.net")
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	err = db.InsertMessage(context.Background(), conn, db.InsertMessageParams{
		ID: id, ConversationID: convID, ContactID: contact.ID,
		Platform: "whatsapp", ExternalConversationID: "ext-conv@s.whatsapp.net",
		ExternalMessageID: extMsgID, ExternalSenderID: "sender@s.whatsapp.net",
		SentAt: sentAt, Kind: kind, Body: body,
		ContextMentionedIDs: "[]",
	})
	if err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

func TestMessageEntryReactionToText(t *testing.T) {
	conn, convID := setupTestDB(t)

	now := time.Date(2026, 3, 14, 9, 12, 30, 0, time.UTC)
	insertMsg(t, conn, convID, "target-1", "ext-target-1", "text", "personal", now)

	reaction := db.Message{
		ID: "react-1", Kind: "reaction", Body: "📚",
		IsFromMe: true, Platform: "whatsapp",
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-target-1"},
		SentAt:          now.Add(time.Second),
	}

	e := messageEntry(reaction, "", conn)

	want := `(📚) (reacting to [09:12:30] Sender: "personal")`
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
	if e.from != "YOU" {
		t.Fatalf("expected sender YOU, got %q", e.from)
	}
}

func TestMessageEntryReactionToMedia(t *testing.T) {
	conn, convID := setupTestDB(t)

	now := time.Date(2026, 3, 14, 9, 12, 30, 0, time.UTC)
	insertMsg(t, conn, convID, "target-2", "ext-target-2", "image", "", now)

	reaction := db.Message{
		ID: "react-2", Kind: "reaction", Body: "❤️",
		IsFromMe: true, Platform: "whatsapp",
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-target-2"},
		SentAt:          now.Add(time.Second),
	}

	e := messageEntry(reaction, "", conn)

	want := "(❤️) (reacting to [09:12:30] Sender: (image))"
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
}

func TestMessageEntryReactionTargetMissing(t *testing.T) {
	conn, _ := setupTestDB(t)

	reaction := db.Message{
		ID: "react-3", Kind: "reaction", Body: "👍",
		IsFromMe: true, Platform: "whatsapp",
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-not-in-db"},
		SentAt:          time.Now(),
	}

	e := messageEntry(reaction, "", conn)

	if e.text != "(👍)" {
		t.Fatalf("expected fallback without target, got %q", e.text)
	}
}

func TestMessageEntryRemovedReaction(t *testing.T) {
	conn, convID := setupTestDB(t)

	now := time.Date(2026, 3, 14, 9, 12, 30, 0, time.UTC)
	insertMsg(t, conn, convID, "target-3", "ext-target-3", "text", "personal", now)

	reaction := db.Message{
		ID: "react-4", Kind: "reaction", Body: "",
		IsFromMe: true, Platform: "whatsapp",
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-target-3"},
		SentAt:          now.Add(time.Second),
	}

	e := messageEntry(reaction, "", conn)

	want := `(removed reaction) (reacting to [09:12:30] Sender: "personal")`
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
}

func TestMessageEntryReactionTruncatesLongBody(t *testing.T) {
	conn, convID := setupTestDB(t)

	now := time.Date(2026, 3, 14, 9, 12, 30, 0, time.UTC)
	longBody := strings.Repeat("a", 250)
	insertMsg(t, conn, convID, "target-4", "ext-target-4", "text", longBody, now)

	reaction := db.Message{
		ID: "react-5", Kind: "reaction", Body: "🔥",
		IsFromMe: true, Platform: "whatsapp",
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-target-4"},
		SentAt:          now.Add(time.Second),
	}

	e := messageEntry(reaction, "", conn)

	want := `(🔥) (reacting to [09:12:30] Sender: "` + longBody[:200] + `…")`
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
}

func TestMessageEntryReplyContext(t *testing.T) {
	conn, convID := setupTestDB(t)

	now := time.Date(2026, 3, 14, 9, 12, 30, 0, time.UTC)
	insertMsg(t, conn, convID, "target-reply", "ext-reply-target", "text", "ok", now)

	reply := db.Message{
		ID: "reply-1", Kind: "text", Body: "where?",
		IsFromMe: false, Platform: "whatsapp",
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-reply-target"},
		SentAt:          now.Add(time.Second),
	}

	e := messageEntry(reply, "Alice", conn)

	want := `where? (replying to [09:12:30] Sender: "ok")`
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
	if e.from != "Alice" {
		t.Fatalf("expected sender Alice, got %q", e.from)
	}
}

func TestMessageEntryReplyTargetMissing(t *testing.T) {
	conn, _ := setupTestDB(t)

	reply := db.Message{
		ID: "reply-2", Kind: "text", Body: "where?",
		IsFromMe: false, Platform: "whatsapp",
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-missing"},
		SentAt:          time.Now(),
	}

	e := messageEntry(reply, "Alice", conn)

	if e.text != "where?" {
		t.Fatalf("expected fallback without target, got %q", e.text)
	}
}

func TestMessageEntryReplyOutOfWindow(t *testing.T) {
	conn, convID := setupTestDB(t)

	oldTime := time.Date(2026, 3, 14, 8, 30, 15, 0, time.UTC)
	now := time.Date(2026, 3, 14, 14, 0, 0, 0, time.UTC)

	insertMsg(t, conn, convID, "old-msg", "ext-old", "text", "how about saturday?", oldTime)

	reply := db.Message{
		ID: "reply-3", Kind: "text", Body: "yes!",
		IsFromMe: false, Platform: "whatsapp",
		ContextStanzaID: sql.NullString{Valid: true, String: "ext-old"},
		SentAt:          now,
	}

	e := messageEntry(reply, "Alice", conn)

	want := `yes! (replying to [08:30:15] Sender: "how about saturday?")`
	if e.text != want {
		t.Fatalf("got %q, want %q", e.text, want)
	}
}

func TestMessageEntryPlainText(t *testing.T) {
	msg := db.Message{
		ID: "plain-1", Kind: "text", Body: "hello",
		IsFromMe: false, SentAt: time.Now(),
	}

	e := messageEntry(msg, "Bob", nil)

	if e.text != "hello" {
		t.Fatalf("expected 'hello', got %q", e.text)
	}
	if e.from != "Bob" {
		t.Fatalf("expected sender Bob, got %q", e.from)
	}
}

func TestRenderUsesSystemMessagesOnly(t *testing.T) {
	conn, convID := setupTestDB(t)
	ctx := context.Background()

	err := db.EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	err = db.InsertSystemMessage(ctx, conn, db.SystemMessageParams{
		ConversationID: convID,
		Kind:           "task_report",
		Body: db.TaskReport{
			TaskID:  "aaaa-bbbb-cccc-dddd",
			Goal:    "check build",
			Status:  "running",
			Content: "working",
		},
		SentAt: time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("insert system message: %v", err)
	}

	cfg := &config.Config{MaxHistory: 10}
	msgSvc := messaging.NewService(cfg, conn, contacts.NewService(conn))
	tl := New(cfg, msgSvc)

	header, rendered, err := tl.Render(ctx, convID)
	if err != nil {
		t.Fatalf("render timeline: %v", err)
	}

	headerOut := strings.Join(header, "\n")
	if !strings.Contains(headerOut, "id: "+convID) {
		t.Fatalf("expected header to include conversation id line, got %q", headerOut)
	}

	full := tl.Get(ctx, convID)
	if !strings.Contains(full, "## Conversation") {
		t.Fatalf("expected timeline output to use conversation header, got %q", full)
	}

	out := strings.Join(rendered, "\n")
	if !strings.Contains(out, "[Task report]") {
		t.Fatalf("expected rendered timeline to include task report, got %q", out)
	}
	if !strings.Contains(out, "goal: check build") {
		t.Fatalf("expected rendered timeline to include task goal, got %q", out)
	}
}
