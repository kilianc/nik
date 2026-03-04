package task

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func testDB(t *testing.T) *Service {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewService(conn)
}

func TestCreateAndGet(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "run build", "step 1\nstep 2", "low")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if task.ID == "" {
		t.Fatal("expected non-empty task ID")
	}
	if task.Status != "pending" {
		t.Fatalf("expected status pending, got %s", task.Status)
	}
	if task.Goal != "run build" {
		t.Fatalf("expected goal 'run build', got %q", task.Goal)
	}

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != task.ID {
		t.Fatalf("expected id %s, got %s", task.ID, got.ID)
	}
	if got.Plan != "step 1\nstep 2" {
		t.Fatalf("expected plan preserved, got %q", got.Plan)
	}
	if got.Thinking != "low" {
		t.Fatalf("expected thinking 'low', got %q", got.Thinking)
	}
}

func TestUpdateStatus(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "test", "", "low")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.UpdateStatus(ctx, task.ID, "running")
	if err != nil {
		t.Fatalf("update status running: %v", err)
	}

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "running" {
		t.Fatalf("expected running, got %s", got.Status)
	}
	if !got.StartedAt.Valid {
		t.Fatal("expected started_at to be set")
	}

	err = svc.UpdateStatus(ctx, task.ID, "completed")
	if err != nil {
		t.Fatalf("update status completed: %v", err)
	}

	got, err = svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if !got.CompletedAt.Valid {
		t.Fatal("expected completed_at to be set")
	}
}

func TestSetActivationID(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "test", "", "low")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, execErr := svc.db.ExecContext(ctx,
		"INSERT INTO activation (id, source, source_id, model, created_at) VALUES (?, 'task', ?, 'test', datetime('now'))",
		"act-123", task.ID)
	if execErr != nil {
		t.Fatalf("insert dummy activation: %v", execErr)
	}

	err = svc.SetActivationID(ctx, task.ID, "act-123")
	if err != nil {
		t.Fatalf("set activation id: %v", err)
	}

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ActivationID != "act-123" {
		t.Fatalf("expected activation id act-123, got %q", got.ActivationID)
	}
}

func TestList(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	svc.Create(ctx, "message", "conv-1", "", "task a", "", "low")
	svc.Create(ctx, "message", "conv-1", "", "task b", "", "low")
	svc.Create(ctx, "message", "conv-2", "", "task c", "", "low")

	tasks, err := svc.List(ctx, "message", "conv-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestReportCRUD(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "test", "", "low")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "result", "build passed")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	reports, err := svc.UnreportedReports(ctx)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Kind != "result" {
		t.Fatalf("expected kind result, got %s", reports[0].Kind)
	}
	if reports[0].Content != "build passed" {
		t.Fatalf("expected content 'build passed', got %q", reports[0].Content)
	}
	if reports[0].Goal != "test" {
		t.Fatalf("expected joined goal 'test', got %q", reports[0].Goal)
	}

	err = svc.MarkReported(ctx, reports[0].ID)
	if err != nil {
		t.Fatalf("mark reported: %v", err)
	}

	reports, err = svc.UnreportedReports(ctx)
	if err != nil {
		t.Fatalf("unreported after mark: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("expected 0 reports after marking, got %d", len(reports))
	}
}

func TestStaleTasks(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	// create a task and mark it running with started_at in the past
	task, err := svc.Create(ctx, "message", "conv-1", "", "stale test", "", "low")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.UpdateStatus(ctx, task.ID, "running")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}

	// with no activation_id, task shouldn't appear as stale
	stale, err := svc.StaleTasks(ctx, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("stale tasks: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected 0 stale tasks without activation_id, got %d", len(stale))
	}

	// set activation_id (normally runner creates the activation first, but for testing
	// we just need the FK to exist -- insert a dummy activation)
	dummyActID := "act-stale-test"
	_, execErr := svc.db.ExecContext(ctx,
		"INSERT INTO activation (id, source, source_id, model, created_at) VALUES (?, 'task', ?, 'test', datetime('now'))",
		dummyActID, task.ID)
	if execErr != nil {
		t.Fatalf("insert dummy activation: %v", execErr)
	}

	err = svc.SetActivationID(ctx, task.ID, dummyActID)
	if err != nil {
		t.Fatalf("set activation id: %v", err)
	}

	// now it should be stale (no tool_calls, started_at is recent but threshold is tiny)
	time.Sleep(5 * time.Millisecond)
	stale, err = svc.StaleTasks(ctx, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("stale tasks: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale task, got %d", len(stale))
	}
	if stale[0].ID != task.ID {
		t.Fatalf("expected stale task %s, got %s", task.ID, stale[0].ID)
	}
}
