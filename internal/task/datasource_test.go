package task

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestDataSourceCheckReturnsReports(t *testing.T) {
	svc, _ := testDB(t)
	ds := NewDataSource(svc, nil)
	ctx := context.Background()

	task, err := svc.Create(ctx, CreateParams{Goal: "test", Thinking: "low", Meta: map[string]string{"conversation_id": "conv-1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "done")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0].Meta["source"] != "task" {
		t.Fatalf("expected source task, got %s", outputs[0].Meta["source"])
	}
	if outputs[0].Meta["conversation_id"] != "conv-1" {
		t.Fatalf("expected conversation_id conv-1, got %s", outputs[0].Meta["conversation_id"])
	}

	err = outputs[0].Processing(ctx)
	if err != nil {
		t.Fatalf("processing: %v", err)
	}

	outputs, err = ds.Check(ctx)
	if err != nil {
		t.Fatalf("check 2: %v", err)
	}
	if len(outputs) != 0 {
		t.Fatalf("expected 0 outputs after marking, got %d", len(outputs))
	}
}

func TestDataSourceStaleDetection(t *testing.T) {
	svc, conn := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, CreateParams{Goal: "stale test", Thinking: "low"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	dummyActID := "act-ds-stale"
	_, execErr := conn.ExecContext(ctx,
		"INSERT INTO activation (id, source, source_id, model, created_at) VALUES (?, 'task', ?, 'test', datetime('now'))",
		dummyActID, task.ID)
	if execErr != nil {
		t.Fatalf("insert dummy activation: %v", execErr)
	}

	err = svc.Start(ctx, task.ID, dummyActID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	stale, err := svc.StaleTasks(ctx, 1*time.Millisecond, 10*time.Minute)
	if err != nil {
		t.Fatalf("stale tasks: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale task, got %d", len(stale))
	}
}

func TestDataSourceStaleSurfacedDirectly(t *testing.T) {
	svc, conn := testDB(t)
	ds := NewDataSource(svc, nil)
	ctx := context.Background()

	tk, err := svc.Create(ctx, CreateParams{Goal: "stale direct", Thinking: "low", Meta: map[string]string{"conversation_id": "conv-1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	dummyActID := "act-stale-direct"
	_, execErr := conn.ExecContext(ctx,
		"INSERT INTO activation (id, source, source_id, model, created_at) VALUES (?, 'task', ?, 'test', datetime('now'))",
		dummyActID, tk.ID)
	if execErr != nil {
		t.Fatalf("insert dummy activation: %v", execErr)
	}

	err = svc.Start(ctx, tk.ID, dummyActID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// backdate started_at and add an old tool call so it's stale by activity
	_, err = conn.ExecContext(ctx, "UPDATE task SET started_at = datetime('now', '-5 minutes') WHERE id = ?", tk.ID)
	if err != nil {
		t.Fatalf("backdate started_at: %v", err)
	}
	_, err = conn.ExecContext(ctx,
		"INSERT INTO tool_call (id, activation_id, name, created_at) VALUES ('tc-1', ?, 'shell', datetime('now', '-5 minutes'))",
		dummyActID)
	if err != nil {
		t.Fatalf("insert old tool call: %v", err)
	}

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 stale output, got %d", len(outputs))
	}
	if outputs[0].Meta["source"] != "task" {
		t.Fatalf("expected source task, got %s", outputs[0].Meta["source"])
	}
	if outputs[0].Meta["conversation_id"] != "conv-1" {
		t.Fatalf("expected conversation_id conv-1, got %s", outputs[0].Meta["conversation_id"])
	}
	if outputs[0].Lines[0] != "[Long-running task]" {
		t.Fatalf("expected long-running header, got %q", outputs[0].Lines[0])
	}

	err = outputs[0].Processed(ctx)
	if err != nil {
		t.Fatalf("processed: %v", err)
	}

	outputs, err = ds.Check(ctx)
	if err != nil {
		t.Fatalf("check 2: %v", err)
	}

	staleOnly := 0
	for _, o := range outputs {
		if len(o.Lines) > 0 && o.Lines[0] == "[Long-running task]" {
			staleOnly++
		}
	}
	if staleOnly != 0 {
		t.Fatalf("expected 0 stale outputs after checked_at, got %d", staleOnly)
	}
}

func TestDataSourceAlarmSourcedWithMeta(t *testing.T) {
	svc, _ := testDB(t)
	ds := NewDataSource(svc, nil)
	ctx := context.Background()

	meta := map[string]string{"conversation_id": "conv-from-alarm"}
	task, err := svc.Create(ctx, CreateParams{Goal: "alarm task", Thinking: "low", Meta: meta})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "alarm done")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0].Meta["conversation_id"] != "conv-from-alarm" {
		t.Fatalf("expected conversation_id conv-from-alarm, got %q", outputs[0].Meta["conversation_id"])
	}
	if outputs[0].Meta["source"] != "task" {
		t.Fatalf("expected source task, got %q", outputs[0].Meta["source"])
	}
}

func TestDataSourceFormatTaskAttention(t *testing.T) {
	ds := &DataSource{}

	a := db.TaskAttention{
		TaskID:  "task-1",
		Goal:    "Run build",
		Status:  "completed",
		Reports: "Build succeeded",
	}

	lines := ds.formatTaskAttention(context.Background(), a)
	if len(lines) == 0 {
		t.Fatal("expected non-empty lines")
	}
	if lines[0] != "[Task completed]" {
		t.Fatalf("expected header [Task completed], got %q", lines[0])
	}

	a.Status = "failed"
	lines = ds.formatTaskAttention(context.Background(), a)
	if lines[0] != "[Task failed]" {
		t.Fatalf("expected header [Task failed], got %q", lines[0])
	}

	a.Status = "running"
	lines = ds.formatTaskAttention(context.Background(), a)
	if lines[0] != "[Task needs attention]" {
		t.Fatalf("expected header [Task needs attention], got %q", lines[0])
	}
}
