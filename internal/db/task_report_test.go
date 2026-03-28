package db

import (
	"context"
	"testing"
	"time"
)

func TestTaskReportList(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-rpt", "")

	taskID := "task-rpt-test"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'build', 'completed', 'low', NOW_ISO8601_MS())",
		taskID, convID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"INSERT INTO task_report (id, task_id, status, content, created_at) VALUES ('rpt1', ?, 'running', 'compiling...', NOW_ISO8601_MS())",
		taskID)
	if err != nil {
		t.Fatalf("insert report 1: %v", err)
	}

	nextReportAt := ISO8601MS(time.Now().Add(time.Second))
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task_report (id, task_id, status, content, created_at) VALUES ('rpt2', ?, 'completed', 'done', ?)",
		taskID, nextReportAt)
	if err != nil {
		t.Fatalf("insert report 2: %v", err)
	}

	reports, err := TaskReportList(ctx, conn, taskID)
	if err != nil {
		t.Fatalf("query reports: %v", err)
	}

	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	if reports[0].Status != "running" {
		t.Fatalf("expected first report status 'running', got %q", reports[0].Status)
	}
	if reports[1].Status != "completed" {
		t.Fatalf("expected second report status 'completed', got %q", reports[1].Status)
	}
}
