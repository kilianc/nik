package task

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
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

type fakeCriticCall struct {
	instructions string
	input        string
}

type fakeCriticCompleter struct {
	actIDs  []string
	results []llm.CompletionResult
	calls   []fakeCriticCall
}

func (f *fakeCriticCompleter) Complete(_ context.Context, instructions string, getInput func() string, _ []llm.ToolDef, _ llm.ToolExecutor) (string, <-chan llm.CompletionResult) {
	input := ""
	if getInput != nil {
		input = getInput()
	}

	f.calls = append(f.calls, fakeCriticCall{
		instructions: instructions,
		input:        input,
	})

	if len(f.actIDs) == 0 || len(f.results) == 0 {
		panic("fakeCriticCompleter exhausted")
	}

	actID := f.actIDs[0]
	f.actIDs = f.actIDs[1:]

	result := f.results[0]
	f.results = f.results[1:]

	ch := make(chan llm.CompletionResult, 1)
	ch <- result
	close(ch)
	return actID, ch
}

func TestRunCriticRetryKeepsPromptContextAndUsesRetryActivation(t *testing.T) {
	svc, conn := testDB(t)
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{
		Goal:           "evaluate build",
		Thinking:       "low",
		ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	for _, actID := range []string{"critic-act-1", "critic-act-2"} {
		_, err = conn.ExecContext(ctx,
			"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'test', datetime('now'))",
			actID,
			testConvID,
		)
		if err != nil {
			t.Fatalf("insert critic activation %s: %v", actID, err)
		}
	}

	fake := &fakeCriticCompleter{
		actIDs: []string{"critic-act-1", "critic-act-2"},
		results: []llm.CompletionResult{
			{Output: "not json"},
			{Output: `{"effectiveness": 4, "tool_feedback": "helped", "skill_feedback": "none", "suggestions": "none"}`},
		},
	}

	runner := NewRunner(&config.Config{
		Models: config.ModelsConfig{
			Critic: config.CriticConfig{Enabled: true},
		},
	}, nil, svc, nil)
	runner.SetCriticLLM(fake)

	runner.RunCritic(ctx, db.Task{
		ID:             task.ID,
		ConversationID: testConvID,
		Goal:           task.Goal,
		Status:         "completed",
	})

	if len(fake.calls) != 2 {
		t.Fatalf("expected 2 critic calls, got %d", len(fake.calls))
	}

	if fake.calls[0].instructions != fake.calls[1].instructions {
		t.Fatal("expected retry to reuse the original critic instructions")
	}

	if fake.calls[0].input != "" {
		t.Fatalf("expected first critic input to stay empty, got %q", fake.calls[0].input)
	}

	if fake.calls[1].input != criticRetryInput {
		t.Fatalf("expected retry input %q, got %q", criticRetryInput, fake.calls[1].input)
	}

	var gotActID string
	var effectiveness int
	err = conn.QueryRowContext(ctx,
		"SELECT activation_id, effectiveness FROM task_assessment WHERE task_id = ?1",
		task.ID,
	).Scan(&gotActID, &effectiveness)
	if err != nil {
		t.Fatalf("query task assessment: %v", err)
	}

	if gotActID != "critic-act-2" {
		t.Fatalf("expected retry activation id critic-act-2, got %s", gotActID)
	}

	if effectiveness != 4 {
		t.Fatalf("expected effectiveness 4, got %d", effectiveness)
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

	t.Run("object valued fields", func(t *testing.T) {
		input := `{"effectiveness": 3, "tool_feedback": {"shell": {"verdict": "helped"}}, "skill_feedback": "ok", "suggestions": ["add build skill"]}`
		out, err := parseCriticOutput(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Effectiveness != 3 {
			t.Fatalf("expected effectiveness 3, got %d", out.Effectiveness)
		}
		if !strings.Contains(out.ToolFeedback, "shell") {
			t.Fatalf("expected tool_feedback to contain 'shell', got %q", out.ToolFeedback)
		}
		if out.SkillFeedback != "ok" {
			t.Fatalf("expected skill_feedback 'ok', got %q", out.SkillFeedback)
		}
		if !strings.Contains(out.Suggestions, "add build skill") {
			t.Fatalf("expected suggestions to contain 'add build skill', got %q", out.Suggestions)
		}
	})
}
