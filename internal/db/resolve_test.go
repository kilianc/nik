package db

import (
	"context"
	"strings"
	"testing"
)

func TestResolveShortID(t *testing.T) {
	t.Run("rejects invalid table", func(t *testing.T) {
		_, err := ResolveShortID(context.Background(), nil, "message; DROP TABLE task", "abc")
		if err == nil {
			t.Fatal("expected error for invalid table")
		}
		if !strings.Contains(err.Error(), "invalid table") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("full UUID passes through without DB", func(t *testing.T) {
		full := "01961234-5678-7000-8000-aabbccddeeff"
		got, err := ResolveShortID(context.Background(), nil, "task", full)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != full {
			t.Fatalf("expected pass-through of full UUID, got %q", got)
		}
	})

	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-resolve", "")

	t.Run("empty short id returns error", func(t *testing.T) {
		_, err := ResolveShortID(ctx, conn, "task", "")
		if err == nil {
			t.Fatal("expected error for empty short id")
		}
		if !strings.Contains(err.Error(), "no task found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	taskID := "01961234-5678-7000-8000-aabbccddeeff"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'test', 'pending', 'low', NOW_ISO8601_MS())",
		taskID, convID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	t.Run("single match resolves", func(t *testing.T) {
		got, err := ResolveShortID(ctx, conn, "task", "ccddeeff")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got != taskID {
			t.Fatalf("expected %q, got %q", taskID, got)
		}
	})

	t.Run("no match returns error", func(t *testing.T) {
		_, err := ResolveShortID(ctx, conn, "task", "ffffffff")
		if err == nil {
			t.Fatal("expected error for no match")
		}
		if !strings.Contains(err.Error(), "no task found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	convID2 := seedConversation(t, ctx, conn, "whatsapp", "ext-resolve-dup", "")
	for _, id := range []string{
		"01961234-5678-7000-8000-000000aaaaaa",
		"01961234-5678-7000-8001-000000aaaaaa",
	} {
		_, err = conn.ExecContext(ctx,
			"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'test', 'pending', 'low', NOW_ISO8601_MS())",
			id, convID2)
		if err != nil {
			t.Fatalf("insert task %s: %v", id, err)
		}
	}

	t.Run("multiple matches returns error", func(t *testing.T) {
		_, err := ResolveShortID(ctx, conn, "task", "aaaaaa")
		if err == nil {
			t.Fatal("expected error for ambiguous match")
		}
		if !strings.Contains(err.Error(), "multiple") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
