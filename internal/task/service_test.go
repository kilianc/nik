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

	ctx := context.Background()

	err = db.EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	_, err = conn.ExecContext(ctx,
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

	task, err := svc.Create(ctx, createParams{
		ConversationID: testConvID,
		Goal:           "run build",
		Plan:           "step 1\nstep 2",
		Thinking:       "low",
	})
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

	task, err := svc.Create(ctx, createParams{Goal: "test", Thinking: "low", ConversationID: testConvID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	actID := "act-start-test"
	_, execErr := conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[]', 'test', NOW_ISO8601_MS())",
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

	var reportContent, reportStatus string
	err = svc.conn.QueryRowContext(ctx,
		`SELECT content, status
		 FROM task_report
		 WHERE task_id = ?1`,
		task.ID,
	).Scan(&reportContent, &reportStatus)
	if err != nil {
		t.Fatalf("query task_report: %v", err)
	}
	if reportContent != "build passed" {
		t.Fatalf("expected content 'build passed', got %q", reportContent)
	}
	if reportStatus != "running" {
		t.Fatalf("expected status 'running', got %q", reportStatus)
	}

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get after report: %v", err)
	}
	if !got.LastReportAt.Valid {
		t.Fatalf("expected last_report_at to be set")
	}

	var content, status string
	err = svc.conn.QueryRowContext(ctx,
		`SELECT json_extract(body, '$.content'),
		        json_extract(body, '$.status')
		 FROM message
		 WHERE platform = 'system'
		   AND kind = 'task_report'
		   AND json_extract(body, '$.task_id') = ?1`,
		task.ID,
	).Scan(&content, &status)
	if err != nil {
		t.Fatalf("query system task report: %v", err)
	}
	if content != "build passed" {
		t.Fatalf("expected system message content 'build passed', got %q", content)
	}
	if status != "running" {
		t.Fatalf("expected system message status 'running', got %q", status)
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
