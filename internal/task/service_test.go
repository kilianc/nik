package task

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kciuffolo/nik/internal/db"
)

const testConvID = "test-conv-001"

func testDB(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	_, err = conn.ExecContext(context.Background(),
		"INSERT INTO conversation (id, platform, external_conversation_id) VALUES (?, 'whatsapp', 'ext-test')",
		testConvID)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	return NewService(conn), conn
}

func TestCreateAndGet(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{Goal: "run build", Plan: "step 1\nstep 2", Thinking: "low"})
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

	task, err := svc.Create(ctx, createParams{Goal: "test", Thinking: "low"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	actID := "act-start-test"
	_, execErr := conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[]', 'test', datetime('now'))",
		actID, testConvID)
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

func TestReportInsertAndList(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{
		Goal:           "test",
		Thinking:       "low",
		ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "running", "build passed")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	reports, err := svc.ListReports(ctx, testConvID, task.CreatedAt)
	if err != nil {
		t.Fatalf("reports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Content != "build passed" {
		t.Fatalf("expected content 'build passed', got %q", reports[0].Content)
	}
	if reports[0].Goal != "test" {
		t.Fatalf("expected goal 'test', got %q", reports[0].Goal)
	}
}

func TestConversationIDRoundTrip(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{
		Goal:           "test meta",
		Thinking:       "low",
		ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if task.ConversationID != testConvID {
		t.Fatalf("expected conversation_id %s, got %q", testConvID, task.ConversationID)
	}

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ConversationID != testConvID {
		t.Fatalf("expected conversation_id %s after get, got %q", testConvID, got.ConversationID)
	}
}

func TestListTasksByConversation(t *testing.T) {
	svc, conn := testDB(t)
	ctx := context.Background()

	conv2 := "test-conv-002"
	_, err := conn.ExecContext(ctx,
		"INSERT INTO conversation (id, platform, external_conversation_id) VALUES (?, 'whatsapp', 'ext-test-2')",
		conv2)
	if err != nil {
		t.Fatalf("seed second conversation: %v", err)
	}

	svc.Create(ctx, createParams{Goal: "task a", Thinking: "low", ConversationID: testConvID})
	svc.Create(ctx, createParams{Goal: "task b", Thinking: "low", ConversationID: testConvID})
	svc.Create(ctx, createParams{Goal: "task c", Thinking: "low", ConversationID: conv2})

	tasks, err := svc.ListTasks(ctx, db.TaskListParams{ConversationID: testConvID})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks for conv-1, got %d", len(tasks))
	}

	tasks, err = svc.ListTasks(ctx, db.TaskListParams{ConversationID: conv2})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task for conv-2, got %d", len(tasks))
	}
}
