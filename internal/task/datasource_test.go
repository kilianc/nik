package task

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestDataSourceCheckReturnsReports(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	ds := NewDataSource(svc, nil)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "test", "", "low")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "result", "done")
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
	if outputs[0].Meta["source"] != "message" {
		t.Fatalf("expected source message, got %s", outputs[0].Meta["source"])
	}

	// processing callback marks reported
	err = outputs[0].Processing(ctx)
	if err != nil {
		t.Fatalf("processing: %v", err)
	}

	// second check should return nothing
	outputs, err = ds.Check(ctx)
	if err != nil {
		t.Fatalf("check 2: %v", err)
	}
	if len(outputs) != 0 {
		t.Fatalf("expected 0 outputs after marking, got %d", len(outputs))
	}
}

func TestDataSourceStaleDetection(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "stale test", "", "low")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// insert a dummy activation
	dummyActID := "act-ds-stale"
	_, execErr := conn.ExecContext(ctx,
		"INSERT INTO activation (id, source, source_id, model, created_at) VALUES (?, 'task', ?, 'test', datetime('now'))",
		dummyActID, task.ID)
	if execErr != nil {
		t.Fatalf("insert dummy activation: %v", execErr)
	}

	err = svc.SetActivationID(ctx, task.ID, dummyActID)
	if err != nil {
		t.Fatalf("set activation id: %v", err)
	}

	err = svc.UpdateStatus(ctx, task.ID, "running")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	stale, err := svc.StaleTasks(ctx, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("stale tasks: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale task, got %d", len(stale))
	}
}

func TestDataSourceStaleSurfacedDirectly(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	ds := NewDataSource(svc, nil)
	ctx := context.Background()

	tk, err := svc.Create(ctx, "message", "conv-1", "", "stale direct", "", "low")
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

	err = svc.SetActivationID(ctx, tk.ID, dummyActID)
	if err != nil {
		t.Fatalf("set activation id: %v", err)
	}

	err = svc.UpdateStatus(ctx, tk.ID, "running")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}

	// manually set started_at in the past so it's stale with a tiny threshold
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
	if outputs[0].Meta["source"] != "message" {
		t.Fatalf("expected source message, got %s", outputs[0].Meta["source"])
	}
	if outputs[0].Lines[0] != "[Stale task]" {
		t.Fatalf("expected stale header, got %q", outputs[0].Lines[0])
	}

	// run Processed callback to set checked_at
	err = outputs[0].Processed(ctx)
	if err != nil {
		t.Fatalf("processed: %v", err)
	}

	// second check should return nothing (checked_at cooldown)
	outputs, err = ds.Check(ctx)
	if err != nil {
		t.Fatalf("check 2: %v", err)
	}

	staleOnly := 0
	for _, o := range outputs {
		if len(o.Lines) > 0 && o.Lines[0] == "[Stale task]" {
			staleOnly++
		}
	}
	if staleOnly != 0 {
		t.Fatalf("expected 0 stale outputs after checked_at, got %d", staleOnly)
	}
}

func TestDataSourceFormatReport(t *testing.T) {
	ds := &DataSource{}

	r := Report{
		ID:      "rpt-1",
		TaskID:  "task-1",
		Kind:    "result",
		Content: "Build succeeded",
		Goal:    "Run build",
		Status:  "completed",
	}

	lines := ds.formatReport(context.Background(), r)
	if len(lines) == 0 {
		t.Fatal("expected non-empty lines")
	}
	if lines[0] != "[Task result]" {
		t.Fatalf("expected header [Task result], got %q", lines[0])
	}

	r.Kind = "error"
	lines = ds.formatReport(context.Background(), r)
	if lines[0] != "[Task error]" {
		t.Fatalf("expected header [Task error], got %q", lines[0])
	}

	r.Kind = "attention"
	lines = ds.formatReport(context.Background(), r)
	if lines[0] != "[Task needs attention]" {
		t.Fatalf("expected header [Task needs attention], got %q", lines[0])
	}
}
