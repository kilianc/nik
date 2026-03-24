package db

import (
	"context"
	"testing"
	"time"
)

func TestSystemMessageInsertWritesSystemRow(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn, "whatsapp", "system-msg@s.whatsapp.net", "dm")
	sentAt := time.Now().UTC().Truncate(time.Second)

	err = SystemMessageInsert(ctx, conn, SystemMessageParams{
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
