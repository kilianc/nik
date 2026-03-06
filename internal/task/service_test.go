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

	task, err := svc.Create(ctx, CreateParams{Goal: "run build", Plan: "step 1\nstep 2", Thinking: "low"})
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

	task, err := svc.Create(ctx, CreateParams{Goal: "test", Thinking: "low"})
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

	task, err := svc.Create(ctx, CreateParams{Goal: "test", Thinking: "low"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "build passed")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	items, err := svc.TasksNeedingAttention(ctx)
	if err != nil {
		t.Fatalf("tasks needing attention: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Reports != "build passed" {
		t.Fatalf("expected reports 'build passed', got %q", items[0].Reports)
	}
	if items[0].Goal != "test" {
		t.Fatalf("expected goal 'test', got %q", items[0].Goal)
	}

	ids := items[0].ReportIDs
	if ids == "" {
		t.Fatal("expected non-empty report IDs")
	}

	err = svc.MarkRead(ctx, ids)
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}

	items, err = svc.TasksNeedingAttention(ctx)
	if err != nil {
		t.Fatalf("attention after mark: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items after marking, got %d", len(items))
	}
}

func TestMetaRoundTrip(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	meta := map[string]string{
		"conversation_id": "conv-123",
		"contact_id":      "contact-456",
	}

	task, err := svc.Create(ctx, CreateParams{Goal: "test meta", Thinking: "low", Meta: meta})
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

	err = svc.InsertReport(ctx, task.ID, "done")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	items, err := svc.TasksNeedingAttention(ctx)
	if err != nil {
		t.Fatalf("tasks needing attention: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Meta["conversation_id"] != "conv-123" {
		t.Fatalf("expected meta conversation_id conv-123, got %q", items[0].Meta["conversation_id"])
	}
}

func TestActiveTasksByConversation(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	svc.Create(ctx, CreateParams{Goal: "task a", Thinking: "low", Meta: map[string]string{"conversation_id": "conv-1"}})
	svc.Create(ctx, CreateParams{Goal: "task b", Thinking: "low", Meta: map[string]string{"conversation_id": "conv-1"}})
	svc.Create(ctx, CreateParams{Goal: "task c", Thinking: "low", Meta: map[string]string{"conversation_id": "conv-2"}})

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

	task, err := svc.Create(ctx, CreateParams{Goal: "stale test", Thinking: "low"})
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

	task, err := svc.Create(ctx, CreateParams{Goal: "long running test", Thinking: "low"})
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
