package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func seedShellActivation(t *testing.T, conn DBTX) string {
	t.Helper()
	ctx := context.Background()

	convID := id.V7()
	_, err := conn.ExecContext(ctx,
		"INSERT INTO conversation (id, platform, external_conversation_id) VALUES (?, 'whatsapp', ?)",
		convID, "ext-"+convID)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	actID := id.V7()
	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:             actID,
		ConversationID: convID,
		Sources:        `["message"]`,
		Model:          "gpt-5",
		CreatedAt:      time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("seed activation: %v", err)
	}

	return actID
}

func TestShellOutputUpsert(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	actID := seedShellActivation(t, conn)

	code := 0
	err = ShellOutputUpsert(ctx, conn, ShellOutputUpsertParams{
		SessionID:    "abc123",
		ActivationID: actID,
		Command:      "echo hi",
		Description:  "test command",
		Output:       "hi",
		ExitCode:     &code,
		Alive:        false,
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
		SessionID:    "def456",
		ActivationID: actID,
		Command:      "sleep 60",
		Description:  "long runner",
		Output:       "waiting...",
		Alive:        true,
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
	err = ShellOutputUpdate(ctx, conn, ShellOutputUpdateParams{
		SessionID: "def456",
		Output:    "done",
		ExitCode:  &exitCode,
		Alive:     false,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	ids, err = ShellOutputAliveIDs(ctx, conn)
	if err != nil {
		t.Fatalf("alive ids after update: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 alive after update, got %d", len(ids))
	}
}

func TestShellOutputActivationIDPreserved(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	actID := seedShellActivation(t, conn)

	err = ShellOutputUpsert(ctx, conn, ShellOutputUpsertParams{
		SessionID:    "sess01",
		ActivationID: actID,
		Command:      "make build",
		Description:  "build for Kevin",
		Output:       "compiling...",
		Alive:        true,
	})
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	var got string
	err = conn.QueryRowContext(ctx,
		"SELECT activation_id FROM shell_output WHERE session_id = 'sess01'").Scan(&got)
	if err != nil {
		t.Fatalf("select after insert: %v", err)
	}
	if got != actID {
		t.Fatalf("expected activation_id %s after insert, got %s", actID, got)
	}

	exitCode := 0
	err = ShellOutputUpdate(ctx, conn, ShellOutputUpdateParams{
		SessionID: "sess01",
		Output:    "done",
		ExitCode:  &exitCode,
		Alive:     false,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	err = conn.QueryRowContext(ctx,
		"SELECT activation_id FROM shell_output WHERE session_id = 'sess01'").Scan(&got)
	if err != nil {
		t.Fatalf("select after update: %v", err)
	}
	if got != actID {
		t.Fatalf("expected activation_id %s preserved after update, got %s", actID, got)
	}
}
