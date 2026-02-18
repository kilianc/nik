package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestJournalHasPageReturnsFalseWhenEmpty(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	has, err := JournalHasPage(ctx, conn, "2026-02-27")
	if err != nil {
		t.Fatalf("check journal page: %v", err)
	}

	if has {
		t.Fatal("expected no page")
	}
}

func TestJournalWritePagePersists(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	date := "2026-02-27"
	err = JournalWritePage(ctx, conn, date, "today was a good day")
	if err != nil {
		t.Fatalf("write journal page: %v", err)
	}

	has, err := JournalHasPage(ctx, conn, date)
	if err != nil {
		t.Fatalf("check journal page: %v", err)
	}
	if !has {
		t.Fatal("expected page to exist after write")
	}

	var content string
	err = conn.QueryRowContext(ctx, "SELECT content FROM journal WHERE date = ?1", date).Scan(&content)
	if err != nil {
		t.Fatalf("query journal content: %v", err)
	}
	if content != "today was a good day" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestJournalConversationsTodayReturnsActiveConversations(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact := seedContactForJournal(t, ctx, conn)
	convID := seedConversation(t, ctx, conn, "whatsapp", "journal-conv@g.us", "group")

	now := time.Now()
	insertTestMessage(t, ctx, conn, insertTestMessageParams{
		ConversationID:         convID,
		ContactID:              contact.ID,
		Platform:               "whatsapp",
		ExternalConversationID: "journal-conv@g.us",
		ExternalMessageID:      "msg-1",
		ExternalSenderID:       "user@s.whatsapp.net",
		SentAt:                 now,
		Body:                   "hello",
	})

	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	convos, err := JournalConversationsToday(ctx, conn, dayStart, dayEnd)
	if err != nil {
		t.Fatalf("journal conversations today: %v", err)
	}

	if len(convos) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(convos))
	}
	if convos[0].ID != convID {
		t.Fatalf("unexpected conversation id: %q", convos[0].ID)
	}
	if convos[0].MessageCount != 1 {
		t.Fatalf("expected 1 message, got %d", convos[0].MessageCount)
	}
}

func TestJournalMessagesTodayReturnsChronological(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact := seedContactForJournal(t, ctx, conn)
	convID := seedConversation(t, ctx, conn, "whatsapp", "journal-msgs@g.us", "dm")

	now := time.Now()
	insertTestMessage(t, ctx, conn, insertTestMessageParams{
		ConversationID:         convID,
		ContactID:              contact.ID,
		Platform:               "whatsapp",
		ExternalConversationID: "journal-msgs@g.us",
		ExternalMessageID:      "first",
		ExternalSenderID:       "user@s.whatsapp.net",
		SentAt:                 now.Add(-time.Hour),
		Body:                   "first",
	})
	insertTestMessage(t, ctx, conn, insertTestMessageParams{
		ConversationID:         convID,
		ContactID:              contact.ID,
		Platform:               "whatsapp",
		ExternalConversationID: "journal-msgs@g.us",
		ExternalMessageID:      "second",
		ExternalSenderID:       "user@s.whatsapp.net",
		SentAt:                 now,
		Body:                   "second",
	})

	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	msgs, err := JournalMessagesToday(ctx, conn, dayStart, dayEnd, 100)
	if err != nil {
		t.Fatalf("journal messages today: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Body != "first" {
		t.Fatalf("expected first message first, got %q", msgs[0].Body)
	}
	if msgs[1].Body != "second" {
		t.Fatalf("expected second message second, got %q", msgs[1].Body)
	}
}

func TestJournalContactsTodayReturnsNewContacts(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	seedContactForJournal(t, ctx, conn)

	now := time.Now()
	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	contacts, err := JournalContactsToday(ctx, conn, dayStart, dayEnd)
	if err != nil {
		t.Fatalf("journal contacts today: %v", err)
	}

	if len(contacts) == 0 {
		t.Fatal("expected at least one contact created today")
	}
	if contacts[0].Name != "Journal Test" {
		t.Fatalf("unexpected contact name: %q", contacts[0].Name)
	}
}

func TestJournalMemoriesTodayReturnsNewMemories(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	id := NewID()
	_, err = conn.ExecContext(ctx, "INSERT INTO memory (id, content) VALUES (?1, ?2)", id, "test memory")
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	now := time.Now()
	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	memories, err := JournalMemoriesToday(ctx, conn, dayStart, dayEnd)
	if err != nil {
		t.Fatalf("journal memories today: %v", err)
	}

	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
	if memories[0].Content != "test memory" {
		t.Fatalf("unexpected memory content: %q", memories[0].Content)
	}
}

func seedContactForJournal(t *testing.T, ctx context.Context, conn *sql.DB) Contact {
	t.Helper()

	contact, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "journal-test@s.whatsapp.net",
		Name:          "Journal Test",
		Phone:         "55555",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact for journal: %v", err)
	}

	return contact
}
