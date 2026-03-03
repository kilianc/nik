package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestToolCallInsertPersistsRows(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	actID := id.V7()
	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:        actID,
		Source:    "messaging",
		Model:     "gpt-5",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	calls := []ToolCallRow{
		{Name: "message_reply", DurationMS: 120, Error: false},
		{Name: "shell", DurationMS: 3400, Error: true},
		{Name: "search_contacts", DurationMS: 45, Error: false},
	}

	err = ToolCallInsert(ctx, conn, actID, calls)
	if err != nil {
		t.Fatalf("insert tool calls: %v", err)
	}

	var count int
	err = conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tool_call WHERE activation_id = ?1", actID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count tool calls: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 tool_call rows, got %d", count)
	}

	var errCount int
	err = conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tool_call WHERE activation_id = ?1 AND error = 1", actID,
	).Scan(&errCount)
	if err != nil {
		t.Fatalf("count error tool calls: %v", err)
	}

	if errCount != 1 {
		t.Fatalf("expected 1 error tool_call, got %d", errCount)
	}
}

func TestToolCallInsertEmptySliceIsNoop(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = ToolCallInsert(ctx, conn, "nonexistent", nil)
	if err != nil {
		t.Fatalf("expected nil error for empty calls, got: %v", err)
	}
}
