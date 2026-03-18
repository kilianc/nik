package brain

import (
	"os"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
)

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

func TestBuildPromptDataBannedWords(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		want  int
	}{
		{"with words", []string{"forbidden", "blocked"}, 2},
		{"empty", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Brain{
				cfg: &config.Config{
					Timezone:    "UTC",
					BannedWords: tt.words,
				},
			}

			data := b.buildPromptData(time.Now(), "")
			if len(data.BannedWords) != tt.want {
				t.Fatalf("expected %d banned words, got %d", tt.want, len(data.BannedWords))
			}
		})
	}
}

func TestBuildPromptDataBreath(t *testing.T) {
	home := t.TempDir()

	b := &Brain{
		cfg: &config.Config{
			Home:     home,
			Timezone: "UTC",
		},
	}

	data := b.buildPromptData(time.Now(), "")
	if data.Breath != "" {
		t.Fatalf("expected empty breath when file missing, got %q", data.Breath)
	}

	breathDir := home + "/breathing"
	if err := os.MkdirAll(breathDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err := os.WriteFile(breathDir+"/latest.md", []byte("  feeling warm today  \n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	data = b.buildPromptData(time.Now(), "")
	if data.Breath != "feeling warm today" {
		t.Fatalf("unexpected breath: %q", data.Breath)
	}
}

func TestBuildPromptDataNow(t *testing.T) {
	b := &Brain{
		cfg: &config.Config{
			Timezone: "UTC",
			Location: "San Francisco",
		},
	}

	data := b.buildPromptData(time.Date(2026, time.January, 2, 15, 4, 0, 0, time.UTC), "")

	if data.Now.Date != "Friday, January 2, 2026 3:04 PM" {
		t.Fatalf("unexpected date: %q", data.Now.Date)
	}
	if data.Now.Timezone != "UTC (UTC, UTC+0)" {
		t.Fatalf("unexpected timezone: %q", data.Now.Timezone)
	}
	if data.Now.Location != "San Francisco" {
		t.Fatalf("unexpected location: %q", data.Now.Location)
	}
}
