package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
