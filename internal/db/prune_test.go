package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func TestPrune(t *testing.T) {
	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	oldTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cutoff := "2026-01-01T00:00:00.000Z"

	err = SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn, "whatsapp", "prune-test-conv", "dm")

	old := seedFullChain(t, ctx, conn, convID, oldTime)
	fresh := seedFullChain(t, ctx, conn, convID, newTime)

	n, err := Prune(ctx, conn, cutoff)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n == 0 {
		t.Fatal("expected rows deleted")
	}

	assertGone(t, conn, "activation", old.activationID)
	assertGone(t, conn, "activation_round", old.roundID)
	assertGone(t, conn, "tool_call", old.toolCallID)
	assertGone(t, conn, "shell_session", old.shellSessionID)
	assertGone(t, conn, "task", old.taskID)
	assertGone(t, conn, "task_report", old.taskReportID)
	assertGone(t, conn, "task_assessment", old.taskAssessmentID)
	assertGone(t, conn, "experiment", old.experimentID)
	assertGone(t, conn, "experiment_variant", old.variantID)
	assertGone(t, conn, "experiment_variant_run", old.runID)
	assertGone(t, conn, "task", old.retryTaskID)

	assertExists(t, conn, "activation", fresh.activationID)
	assertExists(t, conn, "activation_round", fresh.roundID)
	assertExists(t, conn, "tool_call", fresh.toolCallID)
	assertExists(t, conn, "shell_session", fresh.shellSessionID)
	assertExists(t, conn, "task", fresh.taskID)
	assertExists(t, conn, "task_report", fresh.taskReportID)
	assertExists(t, conn, "task_assessment", fresh.taskAssessmentID)
	assertExists(t, conn, "experiment", fresh.experimentID)
	assertExists(t, conn, "experiment_variant", fresh.variantID)
	assertExists(t, conn, "experiment_variant_run", fresh.runID)
	assertExists(t, conn, "task", fresh.retryTaskID)

	assertExists(t, conn, "conversation", convID)

	assertGone(t, conn, "message", old.systemMessageID)
	assertExists(t, conn, "message", fresh.systemMessageID)
}

func TestPruneNoRows(t *testing.T) {
	ctx := context.Background()
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	n, err := Prune(ctx, conn, "2020-01-01T00:00:00.000Z")
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 rows, got %d", n)
	}
}

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"comments only", "-- hello\n-- world\n", 0},
		{"single", "DELETE FROM foo WHERE id = ?1;", 1},
		{"multiple", "DELETE FROM a;\nDELETE FROM b;\n", 2},
		{"with comments", "-- phase 1\nDELETE FROM a;\n-- phase 2\nDELETE FROM b;\n", 2},
		{"prune.sql", queries.Prune, 14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitStatements(tt.input)
			if len(got) != tt.want {
				t.Fatalf("expected %d statements, got %d: %v", tt.want, len(got), got)
			}
		})
	}
}

type chainIDs struct {
	activationID     string
	roundID          string
	toolCallID       string
	shellSessionID   string
	taskID           string
	retryTaskID      string
	taskReportID     string
	taskAssessmentID string
	experimentID     string
	variantID        string
	runID            string
	systemMessageID  string
}

func seedFullChain(t *testing.T, ctx context.Context, conn *sql.DB, convID string, ts time.Time) chainIDs {
	t.Helper()

	tsStr := ts.Format("2006-01-02T15:04:05.000Z")
	c := chainIDs{}

	c.taskID = id.V7()
	err := TaskInsert(ctx, conn, TaskInsertParams{
		ID:             c.taskID,
		ConversationID: convID,
		Goal:           "test task",
		Thinking:       "low",
		Status:         "completed",
		CreatedAt:      ts,
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	c.retryTaskID = id.V7()
	err = TaskInsert(ctx, conn, TaskInsertParams{
		ID:             c.retryTaskID,
		ConversationID: convID,
		RetryForTaskID: c.taskID,
		RetryNumber:    1,
		Goal:           "retry task",
		Thinking:       "low",
		Status:         "completed",
		CreatedAt:      ts,
	})
	if err != nil {
		t.Fatalf("seed retry task: %v", err)
	}

	c.activationID = id.V7()
	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:             c.activationID,
		ConversationID: convID,
		TaskID:         c.taskID,
		Sources:        "[]",
		Model:          "test-model",
		Tools:          "[]",
		ToolSchemas:    "[]",
	})
	if err != nil {
		t.Fatalf("seed activation: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"UPDATE activation SET created_at = ?1 WHERE id = ?2",
		tsStr, c.activationID,
	)
	if err != nil {
		t.Fatalf("backdate activation: %v", err)
	}

	c.roundID, err = ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
		ActivationID: c.activationID,
		Round:        1,
		UserInput:    "hello",
		ModelOutput:  "world",
		Messages:     "[]",
	})
	if err != nil {
		t.Fatalf("seed activation_round: %v", err)
	}

	c.toolCallID = id.V7()
	_, err = conn.ExecContext(ctx,
		"INSERT INTO tool_call (id, activation_id, activation_round_id, name, input, output) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
		c.toolCallID, c.activationID, c.roundID, "test_tool", "{}", "{}",
	)
	if err != nil {
		t.Fatalf("seed tool_call: %v", err)
	}

	c.shellSessionID = id.V7()
	err = ShellSessionUpsert(ctx, conn, ShellSessionUpsertParams{
		ID:           c.shellSessionID,
		ActivationID: c.activationID,
		Command:      "echo hi",
		Output:       "hi",
	})
	if err != nil {
		t.Fatalf("seed shell_session: %v", err)
	}

	c.taskReportID = id.V7()
	err = TaskReportInsert(ctx, conn, TaskReportInsertParams{
		ID:        c.taskReportID,
		TaskID:    c.taskID,
		Status:    "completed",
		Content:   "done",
		CreatedAt: ts,
	})
	if err != nil {
		t.Fatalf("seed task_report: %v", err)
	}

	c.taskAssessmentID = id.V7()
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task_assessment (id, task_id, effectiveness_score, expected_duration_seconds) VALUES (?1, ?2, ?3, ?4)",
		c.taskAssessmentID, c.taskID, 3, 60,
	)
	if err != nil {
		t.Fatalf("seed task_assessment: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"UPDATE task SET activation_id = ?1 WHERE id = ?2",
		c.activationID, c.taskID,
	)
	if err != nil {
		t.Fatalf("link task to activation: %v", err)
	}

	c.experimentID = id.V7()
	err = ExperimentInsert(ctx, conn, ExperimentInsertParams{
		ID:                c.experimentID,
		ActivationRoundID: c.roundID,
		Status:            "complete",
	})
	if err != nil {
		t.Fatalf("seed experiment: %v", err)
	}

	c.variantID = id.V7()
	err = ExperimentVariantInsert(ctx, conn, ExperimentVariantInsertParams{
		ID:           c.variantID,
		ExperimentID: c.experimentID,
		Name:         "baseline",
	})
	if err != nil {
		t.Fatalf("seed experiment_variant: %v", err)
	}

	run, err := ExperimentVariantRunInsert(ctx, conn, c.variantID)
	if err != nil {
		t.Fatalf("seed experiment_variant_run: %v", err)
	}
	c.runID = run.ID

	c.systemMessageID = id.V7()
	_, err = conn.ExecContext(ctx, `
		INSERT INTO message (
			id, conversation_id, contact_id, platform,
			external_conversation_id, external_message_id, external_sender_id,
			sent_at, is_from_me, is_group, kind, body,
			is_edit, context_is_forwarded,
			is_ephemeral, is_view_once, created_at
		) VALUES (
			?, ?, ?, 'system',
			?, ?, ?,
			?, 1, 0, 'task_report', '{}',
			0, 0,
			0, 0, ?
		)`,
		c.systemMessageID, convID, SystemContactID,
		convID, c.systemMessageID, SystemContactID,
		tsStr, tsStr,
	)
	if err != nil {
		t.Fatalf("seed system message: %v", err)
	}

	return c
}

func assertGone(t *testing.T, conn *sql.DB, table, rowID string) {
	t.Helper()

	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE id = ?", rowID).Scan(&count)
	if err != nil {
		t.Fatalf("check %s %s: %v", table, rowID, err)
	}
	if count != 0 {
		t.Fatalf("expected %s %s to be deleted", table, rowID)
	}
}

func assertExists(t *testing.T, conn *sql.DB, table, rowID string) {
	t.Helper()

	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE id = ?", rowID).Scan(&count)
	if err != nil {
		t.Fatalf("check %s %s: %v", table, rowID, err)
	}
	if count != 1 {
		t.Fatalf("expected %s %s to exist", table, rowID)
	}
}
