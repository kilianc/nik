package task

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestBuildToolDocs(t *testing.T) {
	tests := []struct {
		name      string
		tools     []llm.ToolDef
		wantEmpty bool
		wantSubs  []string
	}{
		{
			name:     "with tools",
			tools:    []llm.ToolDef{{Name: "shell", Description: "run commands"}, {Name: "db_query", Description: "query database"}},
			wantSubs: []string{"shell", "db_query"},
		},
		{
			name:      "nil",
			tools:     nil,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildToolDocs(tt.tools)
			if tt.wantEmpty && got != "" {
				t.Fatalf("expected empty, got %q", got)
			}
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Fatalf("expected %q in output", sub)
				}
			}
		})
	}
}

func TestScanTokenTraps(t *testing.T) {
	t.Run("empty home", func(t *testing.T) {
		got := scanTokenTraps(t.TempDir())
		if got != "" {
			t.Fatalf("expected empty output for empty home, got: %q", got)
		}
	})

	home := t.TempDir()

	for _, day := range []string{"12", "13", "14", "15", "16"} {
		dir := filepath.Join(home, "journal", "2026", "03", day)
		os.MkdirAll(dir, 0o755)
		if err := os.WriteFile(filepath.Join(dir, "2026-03-"+day+".md"), []byte("note"), 0o644); err != nil {
			t.Fatalf("write dated journal file: %v", err)
		}
	}
	os.Symlink("2026/03/16/2026-03-16.md", filepath.Join(home, "journal", "latest.md"))

	os.MkdirAll(filepath.Join(home, "soul"), 0o755)
	if err := os.WriteFile(filepath.Join(home, "soul", "latest.md"), []byte("note"), 0o644); err != nil {
		t.Fatalf("write soul latest: %v", err)
	}
	for _, day := range []string{"15", "16"} {
		dir := filepath.Join(home, "soul", "2026", "03", day)
		os.MkdirAll(dir, 0o755)
		if err := os.WriteFile(filepath.Join(dir, "2026-03-"+day+".md"), []byte("note"), 0o644); err != nil {
			t.Fatalf("write dated soul file: %v", err)
		}
	}

	cacheFile := filepath.Join(home, "skills", "google_workspace", "cache", "big.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatalf("mkdir large file dir: %v", err)
	}
	if err := os.WriteFile(cacheFile, bytes.Repeat([]byte("x"), 100*1024), 0o644); err != nil {
		t.Fatalf("write large cache file: %v", err)
	}

	skillDoc := filepath.Join(home, "skills", "google_workspace", "SKILL.md")
	if err := os.WriteFile(skillDoc, bytes.Repeat([]byte("x"), 1024), 0o644); err != nil {
		t.Fatalf("write skill doc: %v", err)
	}

	gitFile := filepath.Join(home, ".git", "big.bin")
	if err := os.MkdirAll(filepath.Dir(gitFile), 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(gitFile, bytes.Repeat([]byte("x"), 200*1024), 0o644); err != nil {
		t.Fatalf("write skipped git file: %v", err)
	}

	denseDir := filepath.Join(home, "skills", "dense_cache")
	if err := os.MkdirAll(denseDir, 0o755); err != nil {
		t.Fatalf("mkdir dense dir: %v", err)
	}
	for i := 0; i < 35; i++ {
		file := filepath.Join(denseDir, "entry-"+fmt.Sprintf("%02d", i)+".txt")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("write dense file: %v", err)
		}
	}

	got := scanTokenTraps(home)

	for _, wanted := range []string{
		"journal/",
		"soul/",
		"2026/03/16/2026-03-16.md",
		"skills/google_workspace/cache/big.json",
		"100 KB",
		"skills/dense_cache/",
		"Dense directories",
	} {
		if !strings.Contains(got, wanted) {
			t.Fatalf("expected token trap output to contain %q, got:\n%s", wanted, got)
		}
	}

	if strings.Contains(got, ".git/") {
		t.Fatalf("expected .git to be skipped:\n%s", got)
	}

	denseSection := extractSection(got, "Dense directories")
	if strings.Contains(denseSection, "journal/") {
		t.Fatalf("dated dir should not appear in dense section:\n%s", got)
	}
	if strings.Contains(denseSection, "soul/") {
		t.Fatalf("dated dir should not appear in dense section:\n%s", got)
	}
}

func TestTaskPromptDataBudget(t *testing.T) {
	tmpl := `{{ .Timeout }} {{ .MaxRounds }}`
	parsed, err := template.New("test").Parse(tmpl)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	data := taskPromptData{
		Timeout:   "1h0m0s",
		MaxRounds: 200,
	}

	var buf strings.Builder
	err = parsed.Execute(&buf, data)
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "1h0m0s") {
		t.Fatalf("expected timeout in output, got: %q", got)
	}
	if !strings.Contains(got, "200") {
		t.Fatalf("expected max rounds in output, got: %q", got)
	}
}

func extractSection(block, header string) string {
	lines := strings.Split(block, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, header) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}

	var section strings.Builder
	for i := start; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			break
		}
		if section.Len() > 0 {
			section.WriteByte('\n')
		}
		section.WriteString(line)
	}

	return section.String()
}
