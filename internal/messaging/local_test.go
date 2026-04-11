package messaging

import (
	"context"
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

	a := NewLocalAdapter(conn)
	ctx := context.Background()

	err = a.StartTyping(ctx, "conv-1")
	if err != nil {
		t.Fatalf("start typing: %v", err)
	}

	s, err := db.SettingGet(ctx, conn, "local_chat_typing")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if s.Value != "true" {
		t.Errorf("expected typing true, got %q", s.Value)
	}

	err = a.StopTyping(ctx, "conv-1")
	if err != nil {
		t.Fatalf("stop typing: %v", err)
	}

	s, err = db.SettingGet(ctx, conn, "local_chat_typing")
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if s.Value != "false" {
		t.Errorf("expected typing false, got %q", s.Value)
	}
}
