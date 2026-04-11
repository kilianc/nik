package messaging

import (
	"context"
	"slices"
	"testing"

	"github.com/kciuffolo/nik/internal/db"
)

func TestLocalAdapterReply(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	a := NewLocalAdapter(conn)
	out, err := a.Reply(context.Background(), "conv-1", "hello", nil)
	if err != nil {
		t.Fatalf("reply: %v", err)
	}

	if out.ExternalMessageID == "" {
		t.Error("expected non-empty external message id")
	}
	if out.ExternalSenderID != db.NikContactID {
		t.Errorf("expected sender %s, got %s", db.NikContactID, out.ExternalSenderID)
	}
	if out.Body != "hello" {
		t.Errorf("expected body 'hello', got %q", out.Body)
	}
	if out.Kind != "text" {
		t.Errorf("expected kind 'text', got %q", out.Kind)
	}
}

func TestLocalAdapterTyping(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	err = db.NikContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure nik contact: %v", err)
	}

	err = db.OwnerContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure owner contact: %v", err)
	}

	err = db.LocalConversationEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure local conversation: %v", err)
	}

	a := NewLocalAdapter(conn)

	err = a.StartTyping(ctx, db.LocalConversationID)
	if err != nil {
		t.Fatalf("start typing: %v", err)
	}

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: db.LocalConversationID})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if !slices.Contains(conv.Activity, "typing") {
		t.Errorf("expected activity to contain 'typing', got %v", conv.Activity)
	}

	err = a.StopTyping(ctx, db.LocalConversationID)
	if err != nil {
		t.Fatalf("stop typing: %v", err)
	}

	conv, err = db.ConversationGet(ctx, conn, db.ConversationGetParams{ID: db.LocalConversationID})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if len(conv.Activity) != 0 {
		t.Errorf("expected empty activity, got %v", conv.Activity)
	}
}
