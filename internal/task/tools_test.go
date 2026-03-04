package task

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestReportHandler(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "test goal", "", "low")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	handler := reportHandler(svc, task.ID)

	args, _ := json.Marshal(reportArgs{
		Note:           "need config file",
		NeedsAttention: true,
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

	reports, err := svc.UnreportedReports(ctx)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Kind != "attention" {
		t.Fatalf("expected kind attention, got %s", reports[0].Kind)
	}
}

func TestReportHandlerNoAttention(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "test goal", "", "low")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	handler := reportHandler(svc, task.ID)

	args, _ := json.Marshal(reportArgs{
		Note:           "halfway done",
		NeedsAttention: false,
	})

	_, err = handler(ctx, llm.ToolCall{
		CallID:    "call-1",
		Name:      "task_report",
		Arguments: string(args),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	reports, err := svc.UnreportedReports(ctx)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Kind != "result" {
		t.Fatalf("expected kind result for no-attention, got %s", reports[0].Kind)
	}
}

func TestCancelHandlerNoRunner(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := NewService(conn)
	ctx := context.Background()

	task, err := svc.Create(ctx, "message", "conv-1", "", "test goal", "", "low")
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
