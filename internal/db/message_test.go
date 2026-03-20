package db

import (
	"context"
	"testing"
	"time"
)

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

	mediaID := "019590a0-0000-7000-8000-000000000010"
	localPath := "media/2026/03/img.jpg"
	describe := "bob holding a camera"
	transcript := "hello from audio"
	err = InsertMedia(ctx, conn, InsertMediaParams{
		ID:             mediaID,
		MimeType:       strPtr("image/jpeg"),
		LocalPath:      &localPath,
		DescribeText:   &describe,
		TranscriptText: &transcript,
	})
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}

	err = UpsertMessageMedia(ctx, conn, messageID, mediaID)
	if err != nil {
		t.Fatalf("upsert message media: %v", err)
	}

	msg, err := GetMessage(ctx, conn, GetMessageParams{ID: messageID})
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
