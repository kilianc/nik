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
	tests := []struct {
		name  string
		calls []db.ToolCallInfo
		want  string
	}{
		{
			"with skills",
			[]db.ToolCallInfo{
				{Name: "load_skill", Input: `{"action":"load","name":"web","reason":"fetch tweet"}`},
				{Name: "shell", Input: `{"action":"run","command":"ls"}`},
				{Name: "load_skill", Input: `{"action":"list","name":""}`},
				{Name: "load_skill", Input: `{"action":"load","name":"journal","reason":"write entry"}`},
			},
			"web, journal",
		},
		{"empty", nil, "(none)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSkillNames(tt.calls)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestFormatToolCalls(t *testing.T) {
	tests := []struct {
		name      string
		calls     []db.ToolCallInfo
		wantEmpty bool
		wantSub   string
	}{
		{
			"with calls",
			[]db.ToolCallInfo{
				{Name: "shell", DurationMS: 120, Error: false},
				{Name: "db_query", DurationMS: 50, Error: true, Output: "no such table: foo"},
			},
			false,
			"",
		},
		{"empty", nil, false, "(no tool calls recorded)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatToolCalls(tt.calls)
			if tt.wantSub != "" && got != tt.wantSub {
				t.Fatalf("expected %q, got %q", tt.wantSub, got)
			}
			if tt.wantEmpty && got != "" {
				t.Fatalf("expected empty, got %q", got)
			}
		})
	}
}

func TestFormatReports(t *testing.T) {
	svc, _ := testDB(t)
	ctx := context.Background()
	runner := NewRunner(&config.Config{}, nil, svc, nil)

	task, err := svc.Create(ctx, createParams{
		Goal: "test", Thinking: "low", ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	got := runner.formatReports(ctx, task.ID)
	if got != "(no reports)" {
		t.Fatalf("expected '(no reports)', got %q", got)
	}

	err = svc.InsertReport(ctx, task.ID, "running", "compiling")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}
	err = svc.InsertReport(ctx, task.ID, "completed", "done")
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	got = runner.formatReports(ctx, task.ID)
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
	if !strings.Contains(got, "effectiveness_score") {
		t.Fatalf("expected effectiveness_score in fallback prompt, got %q", got)
	}
	if !strings.Contains(got, "effectiveness_feedback") {
		t.Fatalf("expected effectiveness_feedback in fallback prompt, got %q", got)
	}
	if !strings.Contains(got, "expected_duration_seconds") {
		t.Fatalf("expected expected_duration_seconds in fallback prompt, got %q", got)
	}
	if !strings.Contains(got, "duration_feedback") {
		t.Fatalf("expected duration_feedback in fallback prompt, got %q", got)
	}
	if !strings.Contains(got, "recommendations") {
		t.Fatalf("expected recommendations in fallback prompt, got %q", got)
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

func (f *fakeCriticCompleter) Complete(_ context.Context, instructions string, getInput func() string, _ []llm.ToolDef, _ llm.ToolExecutor, _ ...llm.CompleteOption) (string, <-chan llm.CompletionResult) {
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
			"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[\"critic\"]', 'test', NOW_ISO8601_MS())",
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
			{Output: `{"effectiveness_score": 4, "effectiveness_feedback": "clean first-try completion", "expected_duration_seconds": 180, "duration_feedback": "on track", "tool_feedback": "helped", "skill_feedback": "none", "recommendations": "none"}`},
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
	var effectivenessScore int
	var expectedDurationSeconds int
	err = conn.QueryRowContext(ctx,
		"SELECT activation_id, effectiveness_score, expected_duration_seconds FROM task_assessment WHERE task_id = ?1",
		task.ID,
	).Scan(&gotActID, &effectivenessScore, &expectedDurationSeconds)
	if err != nil {
		t.Fatalf("query task assessment: %v", err)
	}

	if gotActID != "critic-act-2" {
		t.Fatalf("expected retry activation id critic-act-2, got %s", gotActID)
	}

	if effectivenessScore != 4 {
		t.Fatalf("expected effectiveness_score 4, got %d", effectivenessScore)
	}
	if expectedDurationSeconds != 180 {
		t.Fatalf("expected expected_duration_seconds 180, got %d", expectedDurationSeconds)
	}
}

func TestParseCriticOutput(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		out, err := parseCriticOutput(`{"effectiveness_score": 4, "effectiveness_feedback": "solid first-try result", "expected_duration_seconds": 90, "duration_feedback": "observed 95s vs expected 90s -- roughly equal", "tool_feedback": "shell helped", "skill_feedback": "web not useful", "recommendations": "add build skill"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.EffectivenessScore != 4 {
			t.Fatalf("expected effectiveness_score 4, got %d", out.EffectivenessScore)
		}
		if out.EffectivenessFeedback != "solid first-try result" {
			t.Fatalf("expected effectiveness_feedback 'solid first-try result', got %q", out.EffectivenessFeedback)
		}
		if out.ExpectedDurationSeconds != 90 {
			t.Fatalf("expected expected_duration_seconds 90, got %d", out.ExpectedDurationSeconds)
		}
		if out.DurationFeedback != "observed 95s vs expected 90s -- roughly equal" {
			t.Fatalf("expected duration_feedback 'observed 95s vs expected 90s -- roughly equal', got %q", out.DurationFeedback)
		}
		if out.ToolFeedback != "shell helped" {
			t.Fatalf("expected tool_feedback 'shell helped', got %q", out.ToolFeedback)
		}
		if out.SkillFeedback != "web not useful" {
			t.Fatalf("expected skill_feedback 'web not useful', got %q", out.SkillFeedback)
		}
		if out.Recommendations != "add build skill" {
			t.Fatalf("expected recommendations 'add build skill', got %q", out.Recommendations)
		}
	})

	t.Run("markdown fenced json", func(t *testing.T) {
		input := "```json\n{\"effectiveness_score\": 5, \"effectiveness_feedback\": \"nailed it\", \"expected_duration_seconds\": 30, \"duration_feedback\": \"fast\", \"tool_feedback\": \"ok\", \"skill_feedback\": \"ok\", \"recommendations\": \"none\"}\n```"
		out, err := parseCriticOutput(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.EffectivenessScore != 5 {
			t.Fatalf("expected effectiveness_score 5, got %d", out.EffectivenessScore)
		}
	})

	t.Run("bare fenced json", func(t *testing.T) {
		input := "```\n{\"effectiveness_score\": 3, \"effectiveness_feedback\": \"partial\", \"expected_duration_seconds\": 45, \"duration_feedback\": \"ok\", \"tool_feedback\": \"ok\", \"skill_feedback\": \"ok\", \"recommendations\": \"none\"}\n```"
		out, err := parseCriticOutput(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.EffectivenessScore != 3 {
			t.Fatalf("expected effectiveness_score 3, got %d", out.EffectivenessScore)
		}
	})

	t.Run("effectiveness too low", func(t *testing.T) {
		_, err := parseCriticOutput(`{"effectiveness_score": 0, "expected_duration_seconds": 1, "duration_feedback": "", "tool_feedback": "", "skill_feedback": "", "recommendations": ""}`)
		if err == nil {
			t.Fatal("expected error for effectiveness_score 0")
		}
	})

	t.Run("effectiveness too high", func(t *testing.T) {
		_, err := parseCriticOutput(`{"effectiveness_score": 6, "expected_duration_seconds": 1, "duration_feedback": "", "tool_feedback": "", "skill_feedback": "", "recommendations": ""}`)
		if err == nil {
			t.Fatal("expected error for effectiveness_score 6")
		}
	})

	t.Run("missing expected duration", func(t *testing.T) {
		_, err := parseCriticOutput(`{"effectiveness_score": 4, "tool_feedback": "", "skill_feedback": "", "recommendations": ""}`)
		if err == nil {
			t.Fatal("expected error for missing expected_duration_seconds")
		}
	})

	t.Run("negative expected duration", func(t *testing.T) {
		_, err := parseCriticOutput(`{"effectiveness_score": 4, "expected_duration_seconds": -1, "duration_feedback": "", "tool_feedback": "", "skill_feedback": "", "recommendations": ""}`)
		if err == nil {
			t.Fatal("expected error for negative expected_duration_seconds")
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		_, err := parseCriticOutput(`not json at all`)
		if err == nil {
			t.Fatal("expected error for malformed json")
		}
	})

	t.Run("whitespace padded", func(t *testing.T) {
		out, err := parseCriticOutput(`  {"effectiveness_score": 2, "effectiveness_feedback": "mostly failed", "expected_duration_seconds": 12, "duration_feedback": "ok", "tool_feedback": "x", "skill_feedback": "y", "recommendations": "z"}  `)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.EffectivenessScore != 2 {
			t.Fatalf("expected effectiveness_score 2, got %d", out.EffectivenessScore)
		}
	})

	t.Run("object valued fields", func(t *testing.T) {
		input := `{"effectiveness_score": 3, "effectiveness_feedback": "partial", "expected_duration_seconds": 300, "duration_feedback": "slow -- retries", "tool_feedback": {"shell": {"verdict": "helped"}}, "skill_feedback": "ok", "recommendations": ["add build skill"]}`
		out, err := parseCriticOutput(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.EffectivenessScore != 3 {
			t.Fatalf("expected effectiveness_score 3, got %d", out.EffectivenessScore)
		}
		if !strings.Contains(out.ToolFeedback, "shell") {
			t.Fatalf("expected tool_feedback to contain 'shell', got %q", out.ToolFeedback)
		}
		if out.SkillFeedback != "ok" {
			t.Fatalf("expected skill_feedback 'ok', got %q", out.SkillFeedback)
		}
		if !strings.Contains(out.Recommendations, "add build skill") {
			t.Fatalf("expected recommendations to contain 'add build skill', got %q", out.Recommendations)
		}
	})
}
