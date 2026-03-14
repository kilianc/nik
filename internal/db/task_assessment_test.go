package db

import (
	"context"
	"testing"
)

func TestTaskAssessmentInsertAndQuery(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-assess", "")

	actID := "act-assess-worker"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', datetime('now'))",
		actID, convID)
	if err != nil {
		t.Fatalf("insert worker activation: %v", err)
	}

	taskID := "task-assess-001"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'test goal', 'completed', 'low', datetime('now'))",
		taskID, convID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	criticActID := "act-assess-critic"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'gpt-4.1-nano', datetime('now'))",
		criticActID, convID)
	if err != nil {
		t.Fatalf("insert critic activation: %v", err)
	}

	err = TaskAssessmentInsert(ctx, conn, TaskAssessmentInsertParams{
		TaskID:        taskID,
		ActivationID:  criticActID,
		Effectiveness: 4,
		ToolFeedback:  "shell: helped run tests. db_query: ok.",
		SkillFeedback: "web skill loaded but unused -- wrong skill for the task.",
		Suggestions:   "add a build skill with cached dep resolution.",
	})
	if err != nil {
		t.Fatalf("insert assessment: %v", err)
	}

	var gotEffectiveness int
	var gotToolFeedback, gotSkillFeedback, gotSuggestions string

	err = conn.QueryRowContext(ctx,
		"SELECT effectiveness, tool_feedback, skill_feedback, suggestions FROM task_assessment WHERE task_id = ?", taskID,
	).Scan(&gotEffectiveness, &gotToolFeedback, &gotSkillFeedback, &gotSuggestions)
	if err != nil {
		t.Fatalf("query assessment: %v", err)
	}

	if gotEffectiveness != 4 {
		t.Fatalf("expected effectiveness 4, got %d", gotEffectiveness)
	}
	if gotToolFeedback != "shell: helped run tests. db_query: ok." {
		t.Fatalf("unexpected tool_feedback: %q", gotToolFeedback)
	}
	if gotSkillFeedback != "web skill loaded but unused -- wrong skill for the task." {
		t.Fatalf("unexpected skill_feedback: %q", gotSkillFeedback)
	}
	if gotSuggestions != "add a build skill with cached dep resolution." {
		t.Fatalf("unexpected suggestions: %q", gotSuggestions)
	}
}

func TestTaskAssessmentUniquePerTask(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-assess-dup", "")

	actID := "act-assess-dup"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'gpt-4.1-nano', datetime('now'))",
		actID, convID)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	actID2 := "act-assess-dup-2"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'gpt-4.1-nano', datetime('now'))",
		actID2, convID)
	if err != nil {
		t.Fatalf("insert activation 2: %v", err)
	}

	taskID := "task-assess-dup"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'goal', 'completed', 'low', datetime('now'))",
		taskID, convID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	err = TaskAssessmentInsert(ctx, conn, TaskAssessmentInsertParams{
		TaskID:        taskID,
		ActivationID:  actID,
		Effectiveness: 3,
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err = TaskAssessmentInsert(ctx, conn, TaskAssessmentInsertParams{
		TaskID:        taskID,
		ActivationID:  actID2,
		Effectiveness: 5,
	})
	if err == nil {
		t.Fatal("expected UNIQUE constraint error on second insert for same task_id")
	}
}

func TestTaskAllToolCalls(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-tc", "")

	actID := "act-tc-test"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', datetime('now'))",
		actID, convID)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"INSERT INTO tool_call (id, activation_id, name, input, output, duration_ms, error, created_at) VALUES ('tc1', ?, 'shell', '{\"action\":\"run\"}', 'ok', 100, 0, datetime('now'))",
		actID)
	if err != nil {
		t.Fatalf("insert tool_call 1: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"INSERT INTO tool_call (id, activation_id, name, input, output, duration_ms, error, created_at) VALUES ('tc2', ?, 'db_query', '{\"sql\":\"SELECT 1\"}', 'error', 50, 1, datetime('now', '+1 second'))",
		actID)
	if err != nil {
		t.Fatalf("insert tool_call 2: %v", err)
	}

	calls, err := TaskAllToolCalls(ctx, conn, actID)
	if err != nil {
		t.Fatalf("query tool calls: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}

	if calls[0].Name != "shell" {
		t.Fatalf("expected first call 'shell', got %q", calls[0].Name)
	}
	if calls[0].Error {
		t.Fatal("expected first call no error")
	}

	if calls[1].Name != "db_query" {
		t.Fatalf("expected second call 'db_query', got %q", calls[1].Name)
	}
	if !calls[1].Error {
		t.Fatal("expected second call to have error")
	}
}

func TestTaskReportsByTask(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "ext-rpt", "")

	taskID := "task-rpt-test"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'build', 'completed', 'low', datetime('now'))",
		taskID, convID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"INSERT INTO task_report (id, task_id, status, content, created_at) VALUES ('rpt1', ?, 'running', 'compiling...', datetime('now'))",
		taskID)
	if err != nil {
		t.Fatalf("insert report 1: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"INSERT INTO task_report (id, task_id, status, content, created_at) VALUES ('rpt2', ?, 'completed', 'done', datetime('now', '+1 second'))",
		taskID)
	if err != nil {
		t.Fatalf("insert report 2: %v", err)
	}

	reports, err := TaskReportsByTask(ctx, conn, taskID)
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
