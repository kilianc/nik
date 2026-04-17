package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func TestShiftHeadings(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		content string
		want    string
	}{
		{"shift by 2", 2, "# Title\n## Sub", "### Title\n#### Sub"},
		{"no headings", 1, "plain text", "plain text"},
		{"empty", 1, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shiftHeadings(tt.n, tt.content)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHTMLCommentStripping(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single line", "hello <!-- gone --> world", "hello  world"},
		{"multiline", "before\n<!-- line1\nline2 -->\nafter", "before\nafter"},
		{"multiple", "a <!-- x --> b <!-- y --> c", "a  b  c"},
		{"no comments", "nothing here", "nothing here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlCommentRe.ReplaceAllString(tt.input, "")
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBrainRender(t *testing.T) {
	home := t.TempDir()
	soulDir := filepath.Join(home, "soul")
	os.MkdirAll(soulDir, 0o755)
	os.WriteFile(filepath.Join(soulDir, "latest.md"), []byte("# Soul\n\nI am curious."), 0o644)

	r := NewRenderer(&config.Config{Home: home})

	got := r.Brain(BrainData{
		BannedWords: []string{"forbidden"},
	})

	for _, want := range []string{"You are NIK", "forbidden", "I am curious", "Rules"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, got)
		}
	}

	if strings.Contains(got, "<!--") {
		t.Fatal("expected HTML comments stripped")
	}
}

func TestBrainRenderSections(t *testing.T) {
	r := NewRenderer(&config.Config{Home: t.TempDir()})

	got := r.Brain(BrainData{})

	for _, want := range []string{"Wave 1: Perceive", "Tables (nik.db)", "Plans must be self-contained", "task_spawn"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestBrainRenderNoSoul(t *testing.T) {
	r := NewRenderer(&config.Config{Home: t.TempDir()})

	got := r.Brain(BrainData{})

	if !strings.Contains(got, "You are NIK") {
		t.Fatalf("expected identity section, got:\n%s", got)
	}
}

func TestTaskRender(t *testing.T) {
	r := NewRenderer(&config.Config{Home: t.TempDir()})

	t.Run("without identity", func(t *testing.T) {
		got := r.Task(TaskData{
			Plan:      "do stuff",
			Timeout:   "1h0m0s",
			MaxRounds: 200,
		})

		if strings.Contains(got, "You are NIK") {
			t.Fatalf("expected no identity when profile empty, got:\n%s", got)
		}
	})

	t.Run("with identity", func(t *testing.T) {
		got := r.Task(TaskData{
			Plan:      "do stuff",
			Timeout:   "1h0m0s",
			MaxRounds: 200,
			Profile:   "nik",
		})

		if !strings.Contains(got, "You are NIK") {
			t.Fatalf("expected identity section, got:\n%s", got)
		}
	})
}

func TestTaskRenderSoulFuncMap(t *testing.T) {
	home := t.TempDir()
	soulDir := filepath.Join(home, "soul")
	os.MkdirAll(soulDir, 0o755)
	os.WriteFile(filepath.Join(soulDir, "latest.md"), []byte("# Current soul\n\nI value honesty."), 0o644)

	r := NewRenderer(&config.Config{Home: home})

	got := r.Task(TaskData{
		Plan:      "do stuff",
		Timeout:   "1h0m0s",
		MaxRounds: 200,
		Profile:   "nik",
	})

	if !strings.Contains(got, "### Current soul") {
		t.Fatalf("expected shifted soul headings, got:\n%s", got)
	}
	if !strings.Contains(got, "I value honesty.") {
		t.Fatalf("expected soul body, got:\n%s", got)
	}
}

func TestInputRender(t *testing.T) {
	r := NewRenderer(&config.Config{Home: t.TempDir()})

	t.Run("with recall", func(t *testing.T) {
		got := r.Input(InputData{Recall: "some memories", Timeline: "timeline content"})

		for _, want := range []string{"## What you remember", "some memories", "timeline content"} {
			if !strings.Contains(got, want) {
				t.Fatalf("expected %q in output, got:\n%s", want, got)
			}
		}
	})

	t.Run("no recall", func(t *testing.T) {
		got := r.Input(InputData{Timeline: "timeline content"})

		if strings.Contains(got, "## What you remember") {
			t.Fatalf("expected no recall section, got:\n%s", got)
		}
		if !strings.Contains(got, "timeline content") {
			t.Fatalf("expected timeline, got:\n%s", got)
		}
	})
}

func TestNudgeRender(t *testing.T) {
	r := NewRenderer(&config.Config{Home: t.TempDir()})

	got := r.Nudge("nik-05-retry.md", struct {
		Text        string
		Attempt     int
		MaxAttempts int
	}{Text: "hello world", Attempt: 2, MaxAttempts: 5})

	if !strings.Contains(got, "hello world") {
		t.Fatalf("expected nudge text in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Missing tool call") {
		t.Fatalf("expected nudge header, got:\n%s", got)
	}
	if !strings.Contains(got, "attempt 2/5") {
		t.Fatalf("expected attempt counter in output, got:\n%s", got)
	}

	raw := r.Nudge("task-01-nudge.md", nil)
	if raw == "" {
		t.Fatal("expected non-empty nudge text for raw template")
	}
}
