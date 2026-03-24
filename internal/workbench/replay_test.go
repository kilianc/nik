package workbench

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyPatches(t *testing.T) {
	tests := []struct {
		name         string
		instructions string
		patches      []Patch
		want         string
		wantErr      bool
	}{
		{
			name:         "apply single patch",
			instructions: "Line 1\nRead the timeline carefully.\nLine 3",
			patches: []Patch{{
				File: "prompts/nik-04-brain.md",
				Old:  "Read the timeline carefully.",
				New:  "If ### New is system-only, call message_noop.\nRead the timeline carefully.",
			}},
			want: "Line 1\nIf ### New is system-only, call message_noop.\nRead the timeline carefully.\nLine 3",
		},
		{
			name:         "old text not found",
			instructions: "Line 1\nLine 2",
			patches:      []Patch{{File: "test.md", Old: "nonexistent text", New: "new text"}},
			wantErr:      true,
		},
		{
			name:         "empty patches",
			instructions: "original instructions",
			patches:      nil,
			want:         "original instructions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyPatches(tt.instructions, tt.patches)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected:\n%s\n\ngot:\n%s", tt.want, got)
			}
		})
	}
}

func TestParsePatches(t *testing.T) {
	raw := `[{"file":"prompts/nik-04-brain.md","old":"old text","new":"new text"}]`

	patches, err := ParsePatches(raw)
	if err != nil {
		t.Fatalf("parse patches: %v", err)
	}

	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}

	if patches[0].File != "prompts/nik-04-brain.md" {
		t.Fatalf("expected file %q, got %q", "prompts/nik-04-brain.md", patches[0].File)
	}

	empty, err := ParsePatches("[]")
	if err != nil {
		t.Fatalf("parse empty patches: %v", err)
	}
	if empty != nil {
		t.Fatalf("expected nil patches for empty input, got %v", empty)
	}
}

func TestReplayResultKey(t *testing.T) {
	tests := []struct {
		name  string
		tools []ToolCall
		want  ToolCallKey
	}{
		{
			name:  "no tools",
			tools: nil,
			want:  "no_tools",
		},
		{
			name:  "single tool",
			tools: []ToolCall{{Name: "message_noop"}},
			want:  "message_noop",
		},
		{
			name:  "multiple tools",
			tools: []ToolCall{{Name: "message_send"}, {Name: "task_spawn"}},
			want:  "message_send+task_spawn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ReplayResult{ToolCalls: tt.tools}
			got := r.Key()
			if got != tt.want {
				t.Fatalf("expected key %q, got %q", tt.want, got)
			}
		})
	}
}

func TestComputeDistribution(t *testing.T) {
	t.Run("counts and desired flag", func(t *testing.T) {
		attempts := []AttemptResult{
			{Key: "message_noop", IsDesired: true},
			{Key: "message_send", IsDesired: false},
			{Key: "message_noop", IsDesired: true},
		}

		dist := computeDistribution(attempts, "message_noop")

		byKey := map[string]DistEntry{}
		for _, d := range dist {
			byKey[d.Key] = d
		}

		noop := byKey["message_noop"]
		if noop.Count != 2 {
			t.Fatalf("expected noop count 2, got %d", noop.Count)
		}
		if !noop.IsDesired {
			t.Fatal("expected noop to be desired")
		}

		send := byKey["message_send"]
		if send.Count != 1 {
			t.Fatalf("expected send count 1, got %d", send.Count)
		}
		if send.IsDesired {
			t.Fatal("expected send to not be desired")
		}
	})

	t.Run("empty attempts", func(t *testing.T) {
		dist := computeDistribution(nil, "message_noop")
		if len(dist) != 0 {
			t.Fatalf("expected empty distribution, got %d entries", len(dist))
		}
	})
}

func TestRunReplayResultText(t *testing.T) {
	t.Run("multiple attempts with distribution", func(t *testing.T) {
		r := RunReplayResult{
			Attempts: []AttemptResult{
				{Key: "message_noop", IsDesired: true},
				{Key: "message_send", IsDesired: false},
			},
			Dist: []DistEntry{
				{Key: "message_noop", Count: 1, Percent: 50, IsDesired: true},
				{Key: "message_send", Count: 1, Percent: 50, IsDesired: false},
			},
		}

		text := r.Text()

		if !strings.Contains(text, "attempt 1: message_noop (desired)") {
			t.Fatalf("expected desired tag in text output, got:\n%s", text)
		}
		if !strings.Contains(text, "attempt 2: message_send") {
			t.Fatalf("expected second attempt in text output, got:\n%s", text)
		}
		if !strings.Contains(text, "DISTRIBUTION:") {
			t.Fatalf("expected distribution section in text output, got:\n%s", text)
		}
	})

	t.Run("single attempt omits distribution", func(t *testing.T) {
		r := RunReplayResult{
			Attempts: []AttemptResult{
				{Key: "message_noop", IsDesired: true},
			},
		}

		text := r.Text()

		if strings.Contains(text, "DISTRIBUTION:") {
			t.Fatalf("expected no distribution for single attempt, got:\n%s", text)
		}
	})
}

func TestRunReplayResultJSON(t *testing.T) {
	r := RunReplayResult{
		Attempts: []AttemptResult{
			{
				Result:    ReplayResult{ToolCalls: []ToolCall{{Name: "message_noop"}}, InputTokens: 100, OutputTokens: 50},
				Key:       "message_noop",
				IsDesired: true,
			},
		},
		Dist:    []DistEntry{{Key: "message_noop", Count: 1, Percent: 100, IsDesired: true}},
		Desired: "message_noop",
	}

	out := r.JSON()

	var parsed replayJSONOutput
	err := json.Unmarshal([]byte(out), &parsed)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(parsed.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(parsed.Attempts))
	}
	if parsed.DesiredKey != "message_noop" {
		t.Fatalf("expected desired key %q, got %q", "message_noop", parsed.DesiredKey)
	}
}
