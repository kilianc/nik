package db

import (
	"context"
	"testing"
	"time"
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
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', NOW_ISO8601_MS())",
		actID, convID)
	if err != nil {
		t.Fatalf("insert worker activation: %v", err)
	}

	taskID := "task-assess-001"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'test goal', 'completed', 'low', NOW_ISO8601_MS())",
		taskID, convID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	criticActID := "act-assess-critic"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'gpt-4.1-nano', NOW_ISO8601_MS())",
		criticActID, convID)
	if err != nil {
		t.Fatalf("insert critic activation: %v", err)
	}

	err = TaskAssessmentInsert(ctx, conn, TaskAssessmentInsertParams{
		TaskID:                  taskID,
		ActivationID:            criticActID,
		EffectivenessScore:      4,
		EffectivenessFeedback:   "completed on first try with clean output, one minor formatting issue.",
		ExpectedDurationSeconds: 120,
		DurationFeedback:        "observed 180s vs expected 120s -- 60s overhead from shell retries on flaky test.",
		ToolFeedback:            "shell: helped run tests. db_query: ok.",
		SkillFeedback:           "web skill loaded but unused -- wrong skill for the task.",
		Recommendations:         "add a build skill with cached dep resolution.",
	})
	if err != nil {
		t.Fatalf("insert assessment: %v", err)
	}

	var gotEffectivenessScore int
	var gotExpectedDurationSeconds int
	var gotEffectivenessFeedback, gotDurationFeedback, gotToolFeedback, gotSkillFeedback, gotRecommendations string

	err = conn.QueryRowContext(ctx,
		"SELECT effectiveness_score, effectiveness_feedback, expected_duration_seconds, duration_feedback, tool_feedback, skill_feedback, recommendations FROM task_assessment WHERE task_id = ?",
		taskID,
	).Scan(
		&gotEffectivenessScore,
		&gotEffectivenessFeedback,
		&gotExpectedDurationSeconds,
		&gotDurationFeedback,
		&gotToolFeedback,
		&gotSkillFeedback,
		&gotRecommendations,
	)
	if err != nil {
		t.Fatalf("query assessment: %v", err)
	}

	if gotEffectivenessScore != 4 {
		t.Fatalf("expected effectiveness_score 4, got %d", gotEffectivenessScore)
	}
	if gotEffectivenessFeedback != "completed on first try with clean output, one minor formatting issue." {
		t.Fatalf("unexpected effectiveness_feedback: %q", gotEffectivenessFeedback)
	}
	if gotExpectedDurationSeconds != 120 {
		t.Fatalf("expected expected_duration_seconds 120, got %d", gotExpectedDurationSeconds)
	}
	if gotDurationFeedback != "observed 180s vs expected 120s -- 60s overhead from shell retries on flaky test." {
		t.Fatalf("unexpected duration_feedback: %q", gotDurationFeedback)
	}
	if gotToolFeedback != "shell: helped run tests. db_query: ok." {
		t.Fatalf("unexpected tool_feedback: %q", gotToolFeedback)
	}
	if gotSkillFeedback != "web skill loaded but unused -- wrong skill for the task." {
		t.Fatalf("unexpected skill_feedback: %q", gotSkillFeedback)
	}
	if gotRecommendations != "add a build skill with cached dep resolution." {
		t.Fatalf("unexpected recommendations: %q", gotRecommendations)
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
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'gpt-4.1-nano', NOW_ISO8601_MS())",
		actID, convID)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	actID2 := "act-assess-dup-2"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'gpt-4.1-nano', NOW_ISO8601_MS())",
		actID2, convID)
	if err != nil {
		t.Fatalf("insert activation 2: %v", err)
	}

	taskID := "task-assess-dup"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO task (id, conversation_id, goal, status, thinking, created_at) VALUES (?, ?, 'goal', 'completed', 'low', NOW_ISO8601_MS())",
		taskID, convID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	err = TaskAssessmentInsert(ctx, conn, TaskAssessmentInsertParams{
		TaskID:                  taskID,
		ActivationID:            actID,
		EffectivenessScore:      3,
		ExpectedDurationSeconds: 60,
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err = TaskAssessmentInsert(ctx, conn, TaskAssessmentInsertParams{
		TaskID:                  taskID,
		ActivationID:            actID2,
		EffectivenessScore:      5,
		ExpectedDurationSeconds: 30,
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
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"task\"]', 'gpt-4', NOW_ISO8601_MS())",
		actID, convID)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	_, err = conn.ExecContext(ctx,
		"INSERT INTO tool_call (id, activation_id, name, input, output, duration_ms, error, created_at) VALUES ('tc1', ?, 'shell', '{\"action\":\"run\"}', 'ok', 100, 0, NOW_ISO8601_MS())",
		actID)
	if err != nil {
		t.Fatalf("insert tool_call 1: %v", err)
	}

	nextCreatedAt := ISO8601MS(time.Now().Add(time.Second))
	_, err = conn.ExecContext(ctx,
		"INSERT INTO tool_call (id, activation_id, name, input, output, duration_ms, error, created_at) VALUES ('tc2', ?, 'db_query', '{\"sql\":\"SELECT 1\"}', 'error', 50, 1, ?)",
		actID, nextCreatedAt)
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
