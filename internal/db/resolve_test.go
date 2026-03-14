package db

import (
	"context"
	"strings"
	"testing"
)

func TestResolveShortIDFullUUID(t *testing.T) {
	full := "01961234-5678-7000-8000-aabbccddeeff"

	got, err := ResolveShortID(context.Background(), nil, "task", full)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != full {
		t.Fatalf("expected pass-through of full UUID, got %q", got)
	}
}

func TestResolveShortIDSingleMatch(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-resolve", "")

	taskID := "01961234-5678-7000-8000-aabbccddeeff"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'test', 'pending', 'low', datetime('now'))",
		taskID, convID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	suffix := "ccddeeff"
	got, err := ResolveShortID(ctx, conn, "task", suffix)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != taskID {
		t.Fatalf("expected %q, got %q", taskID, got)
	}
}

func TestResolveShortIDNoMatch(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	_, err = ResolveShortID(ctx, conn, "task", "ffffffff")
	if err == nil {
		t.Fatal("expected error for no match")
	}
	if !strings.Contains(err.Error(), "no task found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveShortIDMultipleMatches(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-resolve-dup", "")

	for _, id := range []string{
		"01961234-5678-7000-8000-000000aaaaaa",
		"01961234-5678-7000-8001-000000aaaaaa",
	} {
		_, err = conn.ExecContext(ctx,
			"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'test', 'pending', 'low', datetime('now'))",
			id, convID)
		if err != nil {
			t.Fatalf("insert task %s: %v", id, err)
		}
	}

	_, err = ResolveShortID(ctx, conn, "task", "aaaaaa")
	if err == nil {
		t.Fatal("expected error for ambiguous match")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("unexpected error: %v", err)
	}
}
