package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestReportHandler(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{
		Goal:           "test goal",
		Thinking:       "low",
		ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	handler := reportHandler(svc, task.ID)

	args, _ := json.Marshal(reportArgs{
		Note: "need config file",
	})

	result, err := handler(ctx, llm.ToolCall{
		CallID:    "call-1",
		Name:      "task_report",
		Arguments: string(args),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty result")
	}

	var reportContent string
	err = svc.conn.QueryRowContext(ctx,
		`SELECT content
		 FROM task_report
		 WHERE task_id = ?1`,
		task.ID,
	).Scan(&reportContent)
	if err != nil {
		t.Fatalf("query task_report: %v", err)
	}
	if reportContent != "need config file" {
		t.Fatalf("expected report content 'need config file', got %q", reportContent)
	}
}

func TestCancelHandlerNoRunner(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{Goal: "test goal", Thinking: "low", ConversationID: testConvID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	runner := &Runner{}
	handler := cancelHandler(svc, runner)

	args, _ := json.Marshal(cancelArgs{TaskID: task.ID, Reason: "user changed their mind"})

	result, err := handler(ctx, llm.ToolCall{
		CallID:    "call-1",
		Name:      "task_cancel",
		Arguments: string(args),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", got.Status)
	}
	if got.CancellationReason != "user changed their mind" {
		t.Fatalf("expected cancellation reason 'user changed their mind', got %q", got.CancellationReason)
	}
}

func TestRetryThinkingOverride(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	original, err := svc.Create(ctx, createParams{
		Goal: "original goal", Thinking: "low", ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	t.Run("explicit override", func(t *testing.T) {
		retry, err := svc.Create(ctx, createParams{
			ConversationID: original.ConversationID,
			RetryForTaskID: original.ID,
			RetryNumber:    1,
			Goal:           original.Goal,
			Plan:           "try harder",
			Thinking:       "high",
		})
		if err != nil {
			t.Fatalf("create retry: %v", err)
		}
		if retry.Thinking != "high" {
			t.Fatalf("expected thinking 'high', got %q", retry.Thinking)
		}
	})

	t.Run("fallback to original", func(t *testing.T) {
		retry, err := svc.Create(ctx, createParams{
			ConversationID: original.ConversationID,
			RetryForTaskID: original.ID,
			RetryNumber:    2,
			Goal:           original.Goal,
			Plan:           "try again",
			Thinking:       original.Thinking,
		})
		if err != nil {
			t.Fatalf("create retry: %v", err)
		}
		if retry.Thinking != "low" {
			t.Fatalf("expected thinking 'low' (inherited), got %q", retry.Thinking)
		}
	})
}

func TestCancelHandlerRequiresReason(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{Goal: "test goal", Thinking: "low", ConversationID: testConvID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	runner := &Runner{}
	handler := cancelHandler(svc, runner)

	args, _ := json.Marshal(cancelArgs{TaskID: task.ID})

	result, err := handler(ctx, llm.ToolCall{
		CallID:    "call-1",
		Name:      "task_cancel",
		Arguments: string(args),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !strings.Contains(result, "reason is required") {
		t.Fatalf("expected error about reason, got %q", result)
	}
}

func TestStatusHandler(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{
		Goal:           "deploy the widget",
		Plan:           "step 1: build\nstep 2: deploy",
		Thinking:       "low",
		ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "running", "building now")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "completed", "deployed successfully")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	handler := statusHandler(svc)
	args, _ := json.Marshal(statusArgs{TaskID: task.ID})
	result, err := handler(ctx, llm.ToolCall{
		CallID:    "call-1",
		Name:      "task_status",
		Arguments: string(args),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var parsed map[string]any
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["task_id"] != id.Shorten(task.ID) {
		t.Errorf("task_id = %v, want %v", parsed["task_id"], id.Shorten(task.ID))
	}
	if parsed["goal"] != "deploy the widget" {
		t.Errorf("goal = %v, want 'deploy the widget'", parsed["goal"])
	}
	if _, ok := parsed["plan"]; ok {
		t.Error("response should not contain plan")
	}
	if _, ok := parsed["reports"]; ok {
		t.Error("response should not contain reports array")
	}
	if _, ok := parsed["retry_chain"]; ok {
		t.Error("response should not contain retry_chain")
	}

	lastReport, ok := parsed["last_report"].(map[string]any)
	if !ok {
		t.Fatalf("last_report missing or wrong type: %v", parsed["last_report"])
	}
	if lastReport["status"] != "completed" {
		t.Errorf("last_report.status = %v, want 'completed'", lastReport["status"])
	}
	if lastReport["note"] != "deployed successfully" {
		t.Errorf("last_report.note = %v, want 'deployed successfully'", lastReport["note"])
	}
}

func TestStatusHandlerTruncatesLongReport(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{
		Goal:           "long report task",
		Thinking:       "low",
		ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	longContent := strings.Repeat("x", 300)
	err = svc.InsertReport(ctx, task.ID, "failed", longContent)
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	handler := statusHandler(svc)
	args, _ := json.Marshal(statusArgs{TaskID: task.ID})
	result, err := handler(ctx, llm.ToolCall{
		CallID:    "call-1",
		Name:      "task_status",
		Arguments: string(args),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var parsed map[string]any
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	lastReport := parsed["last_report"].(map[string]any)
	note := lastReport["note"].(string)
	if !strings.HasSuffix(note, "[truncated]") {
		t.Errorf("expected truncated note, got %q", note)
	}
	if len(note) > statusReportTruncateLen+len(" [truncated]") {
		t.Errorf("note length %d exceeds truncation limit", len(note))
	}
}

func TestStatusHandlerRetryCount(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	original, err := svc.Create(ctx, createParams{
		Goal:           "flaky task",
		Thinking:       "low",
		ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	retry, err := svc.Create(ctx, createParams{
		Goal:           "flaky task",
		Plan:           "try again",
		Thinking:       "low",
		ConversationID: testConvID,
		RetryForTaskID: original.ID,
		RetryNumber:    2,
	})
	if err != nil {
		t.Fatalf("create retry: %v", err)
	}

	handler := statusHandler(svc)
	args, _ := json.Marshal(statusArgs{TaskID: retry.ID})
	result, err := handler(ctx, llm.ToolCall{
		CallID:    "call-1",
		Name:      "task_status",
		Arguments: string(args),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var parsed map[string]any
	err = json.Unmarshal([]byte(result), &parsed)
	if err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	retryCount, ok := parsed["retry_count"].(float64)
	if !ok {
		t.Fatalf("retry_count missing or wrong type: %v", parsed["retry_count"])
	}
	if retryCount != 2 {
		t.Errorf("retry_count = %v, want 2", retryCount)
	}

	args, _ = json.Marshal(statusArgs{TaskID: original.ID})
	result, err = handler(ctx, llm.ToolCall{
		CallID:    "call-2",
		Name:      "task_status",
		Arguments: string(args),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var origParsed map[string]any
	err = json.Unmarshal([]byte(result), &origParsed)
	if err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if _, ok := origParsed["retry_count"]; ok {
		t.Error("original task should not have retry_count")
	}
}
