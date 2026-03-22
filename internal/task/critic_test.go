package task

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
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

	for _, want := range []string{
		"task-123", "run tests", "completed", "shell", "JSON",
		"effectiveness_score", "effectiveness_feedback",
		"expected_duration_seconds", "duration_feedback", "recommendations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in prompt, got:\n%s", want, got)
		}
	}
}

func TestRunCriticNoOp(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
	}{
		{"disabled", &config.Config{}},
		{"nil llm", &config.Config{Models: config.ModelsConfig{Critic: config.CriticConfig{Enabled: true}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := testDB(t)
			runner := NewRunner(tt.cfg, nil, svc, nil)
			runner.RunCritic(t.Context(), db.Task{ID: "task-noop", Goal: "test"})
		})
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
