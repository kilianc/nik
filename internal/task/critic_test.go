package task

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestExtractSkillNames(t *testing.T) {
	calls := []db.ToolCallInfo{
		{Name: "load_skill", Input: `{"action":"load","name":"web","reason":"fetch tweet"}`},
		{Name: "shell", Input: `{"action":"run","command":"ls"}`},
		{Name: "load_skill", Input: `{"action":"list","name":""}`},
		{Name: "load_skill", Input: `{"action":"load","name":"journal","reason":"write entry"}`},
	}

	got := extractSkillNames(calls)
	if got != "web, journal" {
		t.Fatalf("expected 'web, journal', got %q", got)
	}
}

func TestExtractSkillNamesEmpty(t *testing.T) {
	got := extractSkillNames(nil)
	if got != "(none)" {
		t.Fatalf("expected '(none)', got %q", got)
	}
}

func TestFormatToolCalls(t *testing.T) {
	calls := []db.ToolCallInfo{
		{Name: "shell", DurationMS: 120, Error: false},
		{Name: "db_query", DurationMS: 50, Error: true, Output: "no such table: foo"},
	}

	got := formatToolCalls(calls)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestFormatToolCallsEmpty(t *testing.T) {
	got := formatToolCalls(nil)
	if got != "(no tool calls recorded)" {
		t.Fatalf("expected placeholder, got %q", got)
	}
}

func TestFormatReportsNoReports(t *testing.T) {
	svc, _ := testDB(t)

	cfg := &config.Config{}
	runner := NewRunner(cfg, nil, svc, nil)

	task, err := svc.Create(context.Background(), createParams{
		Goal: "test", Thinking: "low", ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	got := runner.formatReports(context.Background(), task.ID)
	if got != "(no reports)" {
		t.Fatalf("expected '(no reports)', got %q", got)
	}
}

func TestFormatReportsWithEntries(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()

	cfg := &config.Config{}
	runner := NewRunner(cfg, nil, svc, nil)

	task, err := svc.Create(ctx, createParams{
		Goal: "test", Thinking: "low", ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "running", "compiling")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	err = svc.InsertReport(ctx, task.ID, "completed", "done")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	got := runner.formatReports(ctx, task.ID)
	if !strings.Contains(got, "running: compiling") {
		t.Fatalf("expected running report, got %q", got)
	}
	if !strings.Contains(got, "completed: done") {
		t.Fatalf("expected completed report, got %q", got)
	}
}

func TestFallbackCriticPrompt(t *testing.T) {
	task := db.Task{ID: "task-123", Goal: "run tests", Status: "completed"}

	got := fallbackCriticPrompt(task, "- shell [ok] 100ms\n", "- [12:00:00] completed: done\n")

	if !strings.Contains(got, "task-123") {
		t.Fatalf("expected task ID in prompt, got %q", got)
	}
	if !strings.Contains(got, "run tests") {
		t.Fatalf("expected goal in prompt, got %q", got)
	}
	if !strings.Contains(got, "completed") {
		t.Fatalf("expected status in prompt, got %q", got)
	}
	if !strings.Contains(got, "shell") {
		t.Fatalf("expected tool calls in prompt, got %q", got)
	}
	if !strings.Contains(got, "JSON") {
		t.Fatalf("expected JSON instruction in fallback prompt, got %q", got)
	}
}

func TestParseCriticOutput(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		out, err := parseCriticOutput(`{"effectiveness": 4, "tool_feedback": "shell helped", "skill_feedback": "web not useful", "suggestions": "add build skill"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Effectiveness != 4 {
			t.Fatalf("expected effectiveness 4, got %d", out.Effectiveness)
		}
		if out.ToolFeedback != "shell helped" {
			t.Fatalf("expected tool_feedback 'shell helped', got %q", out.ToolFeedback)
		}
		if out.SkillFeedback != "web not useful" {
			t.Fatalf("expected skill_feedback 'web not useful', got %q", out.SkillFeedback)
		}
		if out.Suggestions != "add build skill" {
			t.Fatalf("expected suggestions 'add build skill', got %q", out.Suggestions)
		}
	})

	t.Run("markdown fenced json", func(t *testing.T) {
		input := "```json\n{\"effectiveness\": 5, \"tool_feedback\": \"ok\", \"skill_feedback\": \"ok\", \"suggestions\": \"none\"}\n```"
		out, err := parseCriticOutput(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Effectiveness != 5 {
			t.Fatalf("expected effectiveness 5, got %d", out.Effectiveness)
		}
	})

	t.Run("bare fenced json", func(t *testing.T) {
		input := "```\n{\"effectiveness\": 3, \"tool_feedback\": \"ok\", \"skill_feedback\": \"ok\", \"suggestions\": \"none\"}\n```"
		out, err := parseCriticOutput(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Effectiveness != 3 {
			t.Fatalf("expected effectiveness 3, got %d", out.Effectiveness)
		}
	})

	t.Run("effectiveness too low", func(t *testing.T) {
		_, err := parseCriticOutput(`{"effectiveness": 0, "tool_feedback": "", "skill_feedback": "", "suggestions": ""}`)
		if err == nil {
			t.Fatal("expected error for effectiveness 0")
		}
	})

	t.Run("effectiveness too high", func(t *testing.T) {
		_, err := parseCriticOutput(`{"effectiveness": 6, "tool_feedback": "", "skill_feedback": "", "suggestions": ""}`)
		if err == nil {
			t.Fatal("expected error for effectiveness 6")
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		_, err := parseCriticOutput(`not json at all`)
		if err == nil {
			t.Fatal("expected error for malformed json")
		}
	})

	t.Run("whitespace padded", func(t *testing.T) {
		out, err := parseCriticOutput(`  {"effectiveness": 2, "tool_feedback": "x", "skill_feedback": "y", "suggestions": "z"}  `)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Effectiveness != 2 {
			t.Fatalf("expected effectiveness 2, got %d", out.Effectiveness)
		}
	})
}
