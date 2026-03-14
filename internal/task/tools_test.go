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

	reports, err := svc.ListReports(ctx, testConvID, task.CreatedAt)
	if err != nil {
		t.Fatalf("reports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Content != "need config file" {
		t.Fatalf("expected report content 'need config file', got %q", reports[0].Content)
	}
}

func TestAssessHandlerValidation(t *testing.T) {
	svc, conn := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{Goal: "test", Thinking: "low", ConversationID: testConvID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	actID := "act-assess-handler"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'gpt-4.1-nano', datetime('now'))",
		actID, testConvID)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	ctx = context.WithValue(ctx, "meta", map[string]string{
		"activation_id": actID,
	})

	handler := assessHandler(svc, task.ID)

	t.Run("invalid effectiveness", func(t *testing.T) {
		args, _ := json.Marshal(assessArgs{
			Effectiveness: 6,
			ToolFeedback:  "ok",
			SkillFeedback: "ok",
			Suggestions:   "none",
		})

		result, err := handler(ctx, llm.ToolCall{
			CallID:    "test",
			Name:      "task_assess",
			Arguments: string(args),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == "" {
			t.Fatal("expected error result")
		}
	})

	t.Run("valid assessment", func(t *testing.T) {
		args, _ := json.Marshal(assessArgs{
			Effectiveness: 4,
			ToolFeedback:  "shell helped",
			SkillFeedback: "web not useful",
			Suggestions:   "add build skill",
		})

		result, err := handler(ctx, llm.ToolCall{
			CallID:    "test-valid",
			Name:      "task_assess",
			Arguments: string(args),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]any
		if json.Unmarshal([]byte(result), &parsed) != nil {
			t.Fatalf("expected JSON result, got %q", result)
		}
		if parsed["ok"] != true {
			t.Fatalf("expected ok: true, got %v", parsed)
		}

		var effectiveness int
		err = conn.QueryRowContext(ctx,
			"SELECT effectiveness FROM task_assessment WHERE task_id = ?", task.ID,
		).Scan(&effectiveness)
		if err != nil {
			t.Fatalf("query assessment: %v", err)
		}
		if effectiveness != 4 {
			t.Fatalf("expected effectiveness 4, got %d", effectiveness)
		}
	})
}

func TestCancelHandlerNoRunner(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{Goal: "test goal", Thinking: "low"})
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
