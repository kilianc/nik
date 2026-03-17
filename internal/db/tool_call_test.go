package db

import (
	"context"
	"testing"
	"time"
)

func TestToolCallInsertOnePersistsRow(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-tc-insert", "")

	actID := "act-tc-insert"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', NOW_ISO8601_MS())",
		actID, convID)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	err = ToolCallInsertOne(ctx, conn, ToolCallInsertParams{
		ActivationID: actID,
		Name:         "shell",
		Round:        7,
		Input:        `{"action":"run","command":"ls"}`,
		Output:       "file1\nfile2",
		Duration:     150 * time.Millisecond,
		IsError:      false,
	})
	if err != nil {
		t.Fatalf("insert tool call: %v", err)
	}

	var name, input, output string
	var round int
	var durationMS int
	var errFlag int

	err = conn.QueryRowContext(ctx,
		"SELECT name, round, input, output, duration_ms, error FROM tool_call WHERE activation_id = ?", actID,
	).Scan(&name, &round, &input, &output, &durationMS, &errFlag)
	if err != nil {
		t.Fatalf("query tool call: %v", err)
	}

	if name != "shell" {
		t.Fatalf("expected name 'shell', got %q", name)
	}
	if round != 7 {
		t.Fatalf("expected round 7, got %d", round)
	}
	if durationMS != 150 {
		t.Fatalf("expected duration_ms 150, got %d", durationMS)
	}
	if errFlag != 0 {
		t.Fatalf("expected error flag 0, got %d", errFlag)
	}
}

func TestToolCallInsertOneErrorFlag(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-tc-err", "")

	actID := "act-tc-err"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', NOW_ISO8601_MS())",
		actID, convID)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	err = ToolCallInsertOne(ctx, conn, ToolCallInsertParams{
		ActivationID: actID,
		Name:         "db_query",
		Round:        2,
		Input:        `{"sql":"SELECT bad"}`,
		Output:       "no such table",
		Duration:     30 * time.Millisecond,
		IsError:      true,
	})
	if err != nil {
		t.Fatalf("insert tool call: %v", err)
	}

	var errFlag int
	err = conn.QueryRowContext(ctx,
		"SELECT error FROM tool_call WHERE activation_id = ?", actID,
	).Scan(&errFlag)
	if err != nil {
		t.Fatalf("query tool call: %v", err)
	}

	if errFlag != 1 {
		t.Fatalf("expected error flag 1, got %d", errFlag)
	}
}
