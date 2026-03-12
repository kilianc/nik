package db

import (
	"context"
	"testing"
)

func TestShellOutputUpsert(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	code := 0
	err = ShellOutputUpsert(ctx, conn, ShellOutputUpsertParams{
		SessionID:   "abc123",
		Command:     "echo hi",
		Description: "test command",
		Output:      "hi",
		ExitCode:    &code,
		Alive:       false,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	ids, err := ShellOutputAliveIDs(ctx, conn)
	if err != nil {
		t.Fatalf("alive ids: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 alive, got %d", len(ids))
	}

	err = ShellOutputUpsert(ctx, conn, ShellOutputUpsertParams{
		SessionID:   "def456",
		Command:     "sleep 60",
		Description: "long runner",
		Output:      "waiting...",
		Alive:       true,
	})
	if err != nil {
		t.Fatalf("upsert alive: %v", err)
	}

	ids, err = ShellOutputAliveIDs(ctx, conn)
	if err != nil {
		t.Fatalf("alive ids: %v", err)
	}
	if len(ids) != 1 || ids[0] != "def456" {
		t.Fatalf("expected [def456], got %v", ids)
	}

	exitCode := 0
	err = ShellOutputUpsert(ctx, conn, ShellOutputUpsertParams{
		SessionID: "def456",
		Output:    "done",
		ExitCode:  &exitCode,
		Alive:     false,
	})
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	ids, err = ShellOutputAliveIDs(ctx, conn)
	if err != nil {
		t.Fatalf("alive ids after update: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 alive after update, got %d", len(ids))
	}
}
