package task

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestReportHandler(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, CreateParams{Goal: "test goal", Thinking: "low"})
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

	items, err := svc.TasksNeedingAttention(ctx)
	if err != nil {
		t.Fatalf("tasks needing attention: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Reports != "need config file" {
		t.Fatalf("expected report content 'need config file', got %q", items[0].Reports)
	}
}

func TestCancelHandlerNoRunner(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, CreateParams{Goal: "test goal", Thinking: "low"})
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
}
