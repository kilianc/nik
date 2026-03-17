package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func TestOpenInMemoryAppliesSchema(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	var tableName string
	err = conn.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='contact'").Scan(&tableName)
	if err != nil {
		t.Fatalf("query table metadata: %v", err)
	}

	if tableName != "contact" {
		t.Fatalf("expected contact table to exist, got %q", tableName)
	}
}

type insertTestMessageParams struct {
	ConversationID         string
	ContactID              string
	Platform               string
	ExternalConversationID string
	ExternalMessageID      string
	ExternalSenderID       string
	SentAt                 time.Time
	IsFromMe               bool
	Kind                   string
	Body                   string
}

func insertTestMessage(t *testing.T, ctx context.Context, conn *sql.DB, p insertTestMessageParams) string {
	t.Helper()

	if p.Kind == "" {
		p.Kind = "text"
	}
	if p.SentAt.IsZero() {
		p.SentAt = time.Now()
	}

	msgID := id.V7()
	_, err := conn.ExecContext(
		ctx,
		queries.MessageInsert,
		msgID,
		p.ConversationID,
		p.ContactID,
		p.Platform,
		p.ExternalConversationID,
		p.ExternalMessageID,
		p.ExternalSenderID,
		p.SentAt,
		p.IsFromMe,
		false,
		p.Kind,
		p.Body,
		nil,
		false,
		nil,
		nil,
		nil,
		false,
		nil,
		MarshalStringSlice([]string{}),
		false,
		false,
	)
	if err != nil {
		t.Fatalf("insert test message: %v", err)
	}

	row := conn.QueryRowContext(ctx, queries.MessageGet, "", p.Platform, p.ExternalMessageID)
	msg, scanErr := scanMessage(row)
	if scanErr != nil {
		t.Fatalf("read back inserted message: %v", scanErr)
	}

	return msg.ID
}

// seedConversation creates a conversation and returns its canonical ID.
func seedConversation(t *testing.T, ctx context.Context, conn *sql.DB, platform, externalID, kind string) string {
	t.Helper()

	now := time.Now()
	err := UpsertConversation(ctx, conn, UpsertConversationParams{
		Platform:               platform,
		ExternalConversationID: externalID,
		Kind:                   kind,
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := GetConversation(ctx, conn, GetConversationParams{
		Platform:               platform,
		ExternalConversationID: externalID,
	})
	if err != nil {
		t.Fatalf("get seeded conversation: %v", err)
	}

	return conv.ID
}

func TestOpenCreatesDatabasePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "nik.db")

	conn, err := Open(path, nil)
	if err != nil {
		t.Fatalf("open db file: %v", err)
	}
	defer conn.Close()

	_, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat db path: %v", statErr)
	}
}

func TestInsertSystemMessageWritesSystemRow(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn, "whatsapp", "system-msg@s.whatsapp.net", "dm")
	sentAt := time.Now().UTC().Truncate(time.Second)

	err = InsertSystemMessage(ctx, conn, SystemMessageParams{
		ConversationID: convID,
		Kind:           "task_report",
		Body: map[string]any{
			"task_id": "task-123",
			"goal":    "do work",
			"status":  "running",
			"content": "working",
		},
		SentAt: sentAt,
	})
	if err != nil {
		t.Fatalf("insert system message: %v", err)
	}

	var (
		platform               string
		kind                   string
		contactID              string
		externalConversationID string
		externalSenderID       string
	)
	err = conn.QueryRowContext(ctx,
		`SELECT platform, kind, contact_id, external_conversation_id, external_sender_id
		 FROM message
		 WHERE conversation_id = ?1`,
		convID,
	).Scan(&platform, &kind, &contactID, &externalConversationID, &externalSenderID)
	if err != nil {
		t.Fatalf("query inserted message: %v", err)
	}

	if platform != "system" {
		t.Fatalf("expected platform system, got %q", platform)
	}
	if kind != "task_report" {
		t.Fatalf("expected kind task_report, got %q", kind)
	}
	if contactID != SystemContactID {
		t.Fatalf("expected contact_id %q, got %q", SystemContactID, contactID)
	}
	if externalConversationID != convID {
		t.Fatalf("expected external_conversation_id %q, got %q", convID, externalConversationID)
	}
	if externalSenderID != SystemContactID {
		t.Fatalf("expected external_sender_id %q, got %q", SystemContactID, externalSenderID)
	}
}

func TestGetMessageIncludesJoinedMediaFields(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "bob@s.whatsapp.net",
		Name:          "Bob",
		Phone:         "bob",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	now := time.Now()
	err = UpsertConversation(ctx, conn, UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "bob@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := GetConversation(ctx, conn, GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "bob@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	conversationID := conversation.ID

	messageID := insertTestMessage(t, ctx, conn, insertTestMessageParams{
		ConversationID:         conversationID,
		ContactID:              contact.ID,
		Platform:               "whatsapp",
		ExternalConversationID: "bob@s.whatsapp.net",
		ExternalMessageID:      "msg-joined-media",
		ExternalSenderID:       "bob@s.whatsapp.net",
		SentAt:                 now,
		Kind:                   "image",
		Body:                   "photo",
	})

	localPath := "media/whatsapp/bob/img.jpg"
	describe := "bob holding a camera"
	transcript := "hello from audio"
	err = UpsertMedia(ctx, conn, UpsertMediaParams{
		ID:             "media-hash-1",
		MimeType:       strPtr("image/jpeg"),
		LocalPath:      &localPath,
		DescribeText:   &describe,
		TranscriptText: &transcript,
	})
	if err != nil {
		t.Fatalf("upsert media: %v", err)
	}

	err = UpsertMessageMedia(ctx, conn, messageID, "media-hash-1")
	if err != nil {
		t.Fatalf("upsert message media: %v", err)
	}

	msg, err := GetMessage(ctx, conn, GetMessageParams{ID: messageID})
	if err != nil {
		t.Fatalf("get message by id: %v", err)
	}

	if !msg.MediaID.Valid || msg.MediaID.String != "media-hash-1" {
		t.Fatalf("expected media id media-hash-1, got %+v", msg.MediaID)
	}
	if !msg.MediaLocalPath.Valid || msg.MediaLocalPath.String != localPath {
		t.Fatalf("expected local path %q, got %+v", localPath, msg.MediaLocalPath)
	}
	if !msg.MediaDescribeText.Valid || msg.MediaDescribeText.String != describe {
		t.Fatalf("expected describe text %q, got %+v", describe, msg.MediaDescribeText)
	}
	if !msg.MediaTranscriptText.Valid || msg.MediaTranscriptText.String != transcript {
		t.Fatalf("expected transcript text %q, got %+v", transcript, msg.MediaTranscriptText)
	}
}

func TestModelZeroValues(t *testing.T) {
	var contact Contact
	var conversation Conversation
	var message Message
	var alarm Alarm

	if contact.ID != "" {
		t.Fatalf("expected zero contact id")
	}
	if conversation.Platform != "" {
		t.Fatalf("expected zero conversation platform")
	}
	if message.ExternalMessageID != "" {
		t.Fatalf("expected zero external message id")
	}
	if alarm.Goal != "" {
		t.Fatalf("expected zero alarm goal")
	}
}

func strPtr(s string) *string {
	return &s
}
