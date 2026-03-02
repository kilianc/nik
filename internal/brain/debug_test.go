package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestSanitizeSlugNormalizesAndTrims(t *testing.T) {
	got := sanitizeSlug(" Hello, World from Nik! ", 12)
	if got != "hello-world" {
		t.Fatalf("expected sanitized slug hello-world, got %q", got)
	}
}

func TestWriteDebugMarkdownProducesExpectedSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	rec := debugRecord{
		Timestamp: time.Date(2026, 2, 28, 10, 25, 58, 0, time.UTC).Format(time.RFC3339),
		Model:     "gpt-5.2-codex",
		Trigger: map[string]string{
			"source":    "shell",
			"source_id": "abc123",
		},
		Input: debugInput{
			Instructions: "You are nik.",
			UserInput:    "hello nik",
		},
		Tools: []string{"message_reply", "shell"},
		ToolCalls: []debugToolCall{
			{
				Name:   "message_reply",
				Args:   `{"message":"hey"}`,
				Result: `{"sent":true}`,
			},
			{
				Name:   "shell",
				Args:   `{"cmd":"ls"}`,
				Result: "file.txt\ndir/",
				Error:  true,
			},
		},
		Output: debugOutput{
			Raw: "wave 1: thinking\nwave 2: acting",
		},
		Usage: debugUsage{
			InputTokens:  1000,
			OutputTokens: 200,
			TotalTokens:  1200,
			CachedTokens: 600,
			CostUSD:      0.05,
		},
	}

	err := writeDebugMarkdown(path, rec)
	if err != nil {
		t.Fatalf("writeDebugMarkdown: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	md := string(data)

	checks := []struct {
		label string
		want  string
	}{
		{"header", "# Session: 2026-02-28T10:25:58Z"},
		{"model", "**Model:** gpt-5.2-codex"},
		{"trigger", "**Trigger:** shell (`abc123`)"},
		{"cost table header", "| | Tokens | Rate | Cost |"},
		{"input row", "| Input | 400 |"},
		{"cached row", "| Cached | 600 |"},
		{"output row", "| Output | 200 |"},
		{"total row", "| **Total** | **1200** |"},
		{"instructions collapsed", "<details><summary>Instructions (system prompt)</summary>"},
		{"instructions body", "You are nik."},
		{"user input header", "## User Input"},
		{"user input body", "hello nik"},
		{"tools", "`message_reply`, `shell`"},
		{"tool call header", "### 1. message_reply"},
		{"tool call error label", "### 2. shell (error)"},
		{"tool call args json", `"message": "hey"`},
		{"tool call result json", `"sent": true`},
		{"tool call plain result", "file.txt\ndir/"},
		{"thinking header", "### Thinking"},
		{"thinking wave 1", "wave 1: thinking"},
		{"thinking wave 2", "wave 2: acting"},
	}

	for _, c := range checks {
		if !strings.Contains(md, c.want) {
			t.Errorf("%s: expected markdown to contain %q", c.label, c.want)
		}
	}
}

func TestWriteDebugMarkdownRawOutputFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	rec := debugRecord{
		Timestamp: "2026-01-01T00:00:00Z",
		Model:     "m",
		Output: debugOutput{
			Raw: "some raw text",
		},
		Usage: debugUsage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CostUSD:      0.01,
		},
	}

	err := writeDebugMarkdown(path, rec)
	if err != nil {
		t.Fatalf("writeDebugMarkdown: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	md := string(data)

	if !strings.Contains(md, "some raw text") {
		t.Error("expected raw output in markdown")
	}
	if !strings.Contains(md, "### Thinking") {
		t.Error("expected thinking section header")
	}
}

func TestCreateDebugFilePathUsesMdExtension(t *testing.T) {
	dir := t.TempDir()
	ts := time.Date(2026, 2, 28, 10, 25, 58, 0, time.UTC)

	path, err := createDebugFilePath(dir, "hello world", ts)
	if err != nil {
		t.Fatalf("createDebugFilePath: %v", err)
	}

	if !strings.HasSuffix(path, ".md") {
		t.Fatalf("expected .md extension, got %q", path)
	}
}

func TestPreserveNewlinesDoublesSingleNewlines(t *testing.T) {
	input := "line1\nline2\n\nline3\nline4"
	got := preserveNewlines(input)
	want := "line1\n\nline2\n\nline3\n\nline4"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPreserveNewlinesLeavesBlankLinesAlone(t *testing.T) {
	input := "a\n\nb\n\n\nc"
	got := preserveNewlines(input)
	want := "a\n\nb\n\n\nc"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// verify llm import is used (build would fail otherwise, but this makes it explicit)
var _ = llm.ToolDef{}
