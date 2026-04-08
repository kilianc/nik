package db

import (
	"context"
	"testing"
	"time"
)

func TestMessageInsertReturnsFalseOnReplay(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
		Platform:      "whatsapp",
		ExternalID:    "test@s.whatsapp.net",
		Name:          "Test",
		Phone:         "test",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("upsert contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn, "whatsapp", "test-conv@s.whatsapp.net", "dm")
	now := time.Now()

	p := MessageInsertParams{
		ID:                     "aaa-first",
		ConversationID:         convID,
		ContactID:              contact.ID,
		Platform:               "whatsapp",
		ExternalConversationID: "test-conv@s.whatsapp.net",
		ExternalMessageID:      "ext-msg-replay-1",
		ExternalSenderID:       "test@s.whatsapp.net",
		SentAt:                 now,
		Kind:                   "text",
		Body:                   "hello",
		ContextMentionedIDs:    "[]",
	}

	inserted, err := MessageInsert(ctx, conn, p)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if !inserted {
		t.Fatalf("expected first insert to return true")
	}

	p.ID = "bbb-replay"
	p.Body = "updated body"
	inserted, err = MessageInsert(ctx, conn, p)
	if err != nil {
		t.Fatalf("replay insert: %v", err)
	}
	if inserted {
		t.Fatalf("expected replay insert to return false")
	}

	msg, err := MessageGet(ctx, conn, MessageGetParams{
		Platform:          "whatsapp",
		ExternalMessageID: "ext-msg-replay-1",
	})
	if err != nil {
		t.Fatalf("get message: %v", err)
	}

	if msg.ID != "aaa-first" {
		t.Fatalf("expected original id aaa-first, got %s", msg.ID)
	}
	if msg.Body != "hello" {
		t.Fatalf("expected original body hello, got %s", msg.Body)
	}
}

func TestMessageGetIncludesJoinedMediaFields(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact, err := ContactUpsert(ctx, conn, ContactUpsertParams{
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
	err = ConversationUpsert(ctx, conn, ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "bob@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}

	conversation, err := ConversationGet(ctx, conn, ConversationGetParams{
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

	mediaID := "019590a0-0000-7000-8000-000000000010"
	localPath := "media/2026/03/img.jpg"
	describe := "bob holding a camera"
	transcript := "hello from audio"
	err = MediaInsert(ctx, conn, MediaInsertParams{
		ID:             mediaID,
		MimeType:       strPtr("image/jpeg"),
		LocalPath:      &localPath,
		DescribeText:   &describe,
		TranscriptText: &transcript,
	})
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}

	err = MessageMediaUpsert(ctx, conn, messageID, mediaID)
	if err != nil {
		t.Fatalf("upsert message media: %v", err)
	}

	msg, err := MessageGet(ctx, conn, MessageGetParams{ID: messageID})
	if err != nil {
		t.Fatalf("get message by id: %v", err)
	}

	if !msg.MediaID.Valid || msg.MediaID.String != mediaID {
		t.Fatalf("expected media id %s, got %+v", mediaID, msg.MediaID)
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
