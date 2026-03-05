package task

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func testDB(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewService(conn), conn
}

func TestCreateAndGet(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "", "run build", "step 1\nstep 2", "low", nil)
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

func TestStartAndComplete(t *testing.T) {
	svc, conn := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "", "test", "", "low", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	actID := "act-start-test"
	_, execErr := conn.ExecContext(ctx,
		"INSERT INTO activation (id, source, source_id, model, created_at) VALUES (?, 'task', ?, 'test', datetime('now'))",
		actID, task.ID)
	if execErr != nil {
		t.Fatalf("insert dummy activation: %v", execErr)
	}

	err = svc.Start(ctx, task.ID, actID)
	if err != nil {
		t.Fatalf("start: %v", err)
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
	if got.ActivationID != actID {
		t.Fatalf("expected activation id %s, got %q", actID, got.ActivationID)
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

func TestReportCRUD(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "", "test", "", "low", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "result", "build passed")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	reports, err := svc.UnreadReports(ctx)
	if err != nil {
		t.Fatalf("unread: %v", err)
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

	err = svc.MarkRead(ctx, reports[0].ID)
	if err != nil {
		t.Fatalf("mark reported: %v", err)
	}

	reports, err = svc.UnreadReports(ctx)
	if err != nil {
		t.Fatalf("unread after mark: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("expected 0 reports after marking, got %d", len(reports))
	}
}

func TestMetaRoundTrip(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	meta := map[string]string{
		"conversation_id": "conv-123",
		"contact_id":      "contact-456",
	}

	task, err := svc.Create(ctx, "", "test meta", "", "low", meta)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if task.Meta["conversation_id"] != "conv-123" {
		t.Fatalf("expected conversation_id conv-123, got %q", task.Meta["conversation_id"])
	}

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Meta["conversation_id"] != "conv-123" {
		t.Fatalf("expected conversation_id conv-123 after get, got %q", got.Meta["conversation_id"])
	}
	if got.Meta["contact_id"] != "contact-456" {
		t.Fatalf("expected contact_id contact-456 after get, got %q", got.Meta["contact_id"])
	}

	err = svc.InsertReport(ctx, task.ID, "result", "done")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	reports, err := svc.UnreadReports(ctx)
	if err != nil {
		t.Fatalf("unread: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Meta["conversation_id"] != "conv-123" {
		t.Fatalf("expected report meta conversation_id conv-123, got %q", reports[0].Meta["conversation_id"])
	}
}

func TestActiveTasksByConversation(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	svc.Create(ctx, "", "task a", "", "low", map[string]string{"conversation_id": "conv-1"})
	svc.Create(ctx, "", "task b", "", "low", map[string]string{"conversation_id": "conv-1"})
	svc.Create(ctx, "", "task c", "", "low", map[string]string{"conversation_id": "conv-2"})

	active, err := svc.ActiveTasks(ctx, "conv-1")
	if err != nil {
		t.Fatalf("active tasks: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active tasks for conv-1, got %d", len(active))
	}

	active, err = svc.ActiveTasks(ctx, "conv-2")
	if err != nil {
		t.Fatalf("active tasks: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active task for conv-2, got %d", len(active))
	}
}

func TestStaleTasks(t *testing.T) {
	svc, conn := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "", "stale test", "", "low", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	dummyActID := "act-stale-test"
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

	// now it should be stale (no tool_calls, started_at is recent but threshold is tiny)
	time.Sleep(5 * time.Millisecond)
	stale, err := svc.StaleTasks(ctx, 1*time.Millisecond, 10*time.Minute)
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

func TestStaleTasksLongRunning(t *testing.T) {
	svc, conn := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, "", "long running test", "", "low", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	dummyActID := "act-long-running"
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

	// add a recent tool call so it's NOT stale by activity
	_, err = conn.ExecContext(ctx,
		"INSERT INTO tool_call (id, activation_id, name, created_at) VALUES ('tc-lr', ?, 'shell', datetime('now'))",
		dummyActID)
	if err != nil {
		t.Fatalf("insert tool call: %v", err)
	}

	// should NOT be stale with a large stale threshold and large max running
	stale, err := svc.StaleTasks(ctx, 10*time.Minute, 10*time.Minute)
	if err != nil {
		t.Fatalf("stale tasks: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected 0 stale tasks, got %d", len(stale))
	}

	// backdate started_at so it exceeds maxRunning threshold
	_, err = conn.ExecContext(ctx, "UPDATE task SET started_at = datetime('now', '-15 minutes') WHERE id = ?", task.ID)
	if err != nil {
		t.Fatalf("backdate started_at: %v", err)
	}

	// now should appear because started_at is 15 min ago and maxRunning is 10 min
	stale, err = svc.StaleTasks(ctx, 10*time.Minute, 10*time.Minute)
	if err != nil {
		t.Fatalf("stale tasks: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale task via long-running trigger, got %d", len(stale))
	}
}
