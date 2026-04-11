package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestMediaInsertValidatesID(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = MediaInsert(ctx, conn, MediaInsertParams{})
	if err == nil {
		t.Fatalf("expected error for empty media id")
	}
	if !strings.Contains(err.Error(), "empty media id") {
		t.Fatalf("unexpected error: %v", err)
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

	mediaID := "019590a0-0000-7000-8000-000000000002"
	mimeType := "image/jpeg"
	localPath := "media/2026/03/resolve-test.jpg"
	err = MediaInsert(ctx, conn, MediaInsertParams{
		ID:        mediaID,
		MimeType:  &mimeType,
		LocalPath: &localPath,
	})
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}

	err = MessageMediaUpsert(ctx, conn, messageID, mediaID)
	if err != nil {
		t.Fatalf("upsert message_media: %v", err)
	}

	r, err := MediaResolveByPath(ctx, conn, localPath)
	if err != nil {
		t.Fatalf("resolve by path: %v", err)
	}

	if r.MediaID != mediaID {
		t.Fatalf("expected media id %s, got %s", mediaID, r.MediaID)
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

	mediaID := "019590a0-0000-7000-8000-000000000003"
	mimeType := "audio/ogg"
	localPath := "media/2026/03/transcript-test.ogg"
	err = MediaInsert(ctx, conn, MediaInsertParams{
		ID:        mediaID,
		MimeType:  &mimeType,
		LocalPath: &localPath,
	})
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}

	now := time.Now()
	transcript := "hello from audio"
	rows, err := MediaUpdate(ctx, conn, MediaUpdateParams{
		ID:             mediaID,
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
		mediaID,
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

	contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
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
