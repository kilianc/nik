package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestUpsertMediaValidatesID(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = UpsertMedia(ctx, conn, UpsertMediaParams{})
	if err == nil {
		t.Fatalf("expected error for empty media id")
	}
	if !strings.Contains(err.Error(), "empty media id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMessageMediaRoundTrip(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	messageID := seedMessageForMediaTest(t, ctx, conn)

	mimeType := "image/jpeg"
	localPath := "media/hash.jpg"
	err = UpsertMedia(ctx, conn, UpsertMediaParams{
		ID:        "hash-123",
		MimeType:  &mimeType,
		LocalPath: &localPath,
	})
	if err != nil {
		t.Fatalf("upsert media: %v", err)
	}

	err = UpsertMessageMedia(ctx, conn, messageID, "hash-123")
	if err != nil {
		t.Fatalf("upsert message_media: %v", err)
	}

	var rowID string
	err = conn.QueryRowContext(ctx,
		"SELECT id FROM message_media WHERE message_id = ?1",
		messageID,
	).Scan(&rowID)
	if err != nil {
		t.Fatalf("query message_media id: %v", err)
	}
	if rowID == "" {
		t.Fatalf("expected non-empty message_media id")
	}

	msg, err := GetMessage(ctx, conn, GetMessageParams{ID: messageID})
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if !msg.MediaID.Valid || msg.MediaID.String != "hash-123" {
		t.Fatalf("expected media id hash-123, got %+v", msg.MediaID)
	}
}

func TestMediaResolveByPath(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	messageID := seedMessageForMediaTest(t, ctx, conn)

	mimeType := "image/jpeg"
	localPath := "media/resolve-test.jpg"
	err = UpsertMedia(ctx, conn, UpsertMediaParams{
		ID:        "resolve-media-1",
		MimeType:  &mimeType,
		LocalPath: &localPath,
	})
	if err != nil {
		t.Fatalf("upsert media: %v", err)
	}

	err = UpsertMessageMedia(ctx, conn, messageID, "resolve-media-1")
	if err != nil {
		t.Fatalf("upsert message_media: %v", err)
	}

	r, err := MediaResolveByPath(ctx, conn, "media/resolve-test.jpg")
	if err != nil {
		t.Fatalf("resolve by path: %v", err)
	}

	if r.MediaID != "resolve-media-1" {
		t.Fatalf("expected media id resolve-media-1, got %s", r.MediaID)
	}
	if r.MessageID != messageID {
		t.Fatalf("expected message id %s, got %s", messageID, r.MessageID)
	}
	if r.ConversationID == "" {
		t.Fatalf("expected non-empty conversation id")
	}
	if r.Platform != "whatsapp" {
		t.Fatalf("expected platform whatsapp, got %s", r.Platform)
	}
	if r.ExternalMessageID != "media-msg-1" {
		t.Fatalf("expected external message id media-msg-1, got %s", r.ExternalMessageID)
	}

	_, err = MediaResolveByPath(ctx, conn, "media/nonexistent.jpg")
	if err == nil {
		t.Fatalf("expected error for nonexistent path")
	}
}

func TestMediaUpdate(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	mimeType := "audio/ogg"
	localPath := "media/transcript-test.ogg"
	err = UpsertMedia(ctx, conn, UpsertMediaParams{
		ID:        "transcript-media-1",
		MimeType:  &mimeType,
		LocalPath: &localPath,
	})
	if err != nil {
		t.Fatalf("upsert media: %v", err)
	}

	now := time.Now()
	transcript := "hello from audio"
	rows, err := MediaUpdate(ctx, conn, MediaUpdateParams{
		ID:             "transcript-media-1",
		TranscriptText: &transcript,
		TranscribedAt:  &now,
	})
	if err != nil {
		t.Fatalf("update transcript: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row affected, got %d", rows)
	}

	var transcriptText string
	var transcribedAt string
	err = conn.QueryRowContext(ctx,
		"SELECT transcript_text, transcribed_at FROM media WHERE id = ?1",
		"transcript-media-1",
	).Scan(&transcriptText, &transcribedAt)
	if err != nil {
		t.Fatalf("query transcript: %v", err)
	}
	if transcriptText != "hello from audio" {
		t.Fatalf("expected transcript 'hello from audio', got %q", transcriptText)
	}
	if transcribedAt == "" {
		t.Fatalf("expected non-empty transcribed_at")
	}
}

func seedMessageForMediaTest(t *testing.T, ctx context.Context, conn *sql.DB) string {
	t.Helper()

	contact, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "media@s.whatsapp.net",
		Name:          "Media Tester",
		Phone:         "55555",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	now := time.Now()
	convID := seedConversation(t, ctx, conn, "whatsapp", "media@s.whatsapp.net", "dm")

	return insertTestMessage(t, ctx, conn, insertTestMessageParams{
		ConversationID:         convID,
		ContactID:              contact.ID,
		Platform:               "whatsapp",
		ExternalConversationID: "media@s.whatsapp.net",
		ExternalMessageID:      "media-msg-1",
		ExternalSenderID:       "media@s.whatsapp.net",
		SentAt:                 now,
		Kind:                   "image",
		Body:                   "caption",
	})
}
