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
}
