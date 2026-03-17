package task

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestBuildToolDocs(t *testing.T) {
	tools := []llm.ToolDef{
		{Name: "shell", Description: "run commands"},
		{Name: "db_query", Description: "query database"},
	}

	got := buildToolDocs(tools)
	if got == "" {
		t.Fatal("expected non-empty tool docs")
	}

	for _, name := range []string{"shell", "db_query"} {
		if !strings.Contains(got, name) {
			t.Fatalf("expected tool docs to contain %q", name)
		}
	}
}

func TestBuildToolDocsEmpty(t *testing.T) {
	got := buildToolDocs(nil)
	if got != "" {
		t.Fatalf("expected empty string for no tools, got %q", got)
	}
}

func TestScanTokenTraps(t *testing.T) {
	home := t.TempDir()

	journal := filepath.Join(home, "journal")
	os.MkdirAll(journal, 0o755)
	for _, file := range []string{
		"2026-03-12.md",
		"2026-03-13.md",
		"2026-03-14.md",
		"2026-03-15.md",
		"2026-03-16.md",
	} {
		if err := os.WriteFile(filepath.Join(journal, file), []byte("note"), 0o644); err != nil {
			t.Fatalf("write dated journal file: %v", err)
		}
	}

	soul := filepath.Join(home, "soul")
	os.MkdirAll(soul, 0o755)
	for _, file := range []string{
		"latest.md",
		"2026-03-15.md",
		"2026-03-16.md",
	} {
		if err := os.WriteFile(filepath.Join(soul, file), []byte("note"), 0o644); err != nil {
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
		"2026-03-16.md",
		"latest.md",
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

func TestScanTokenTrapsEmptyHome(t *testing.T) {
	home := t.TempDir()

	got := scanTokenTraps(home)
	if got != "" {
		t.Fatalf("expected empty output for empty home, got: %q", got)
	}
}

func TestRunCriticNoOpWhenDisabled(t *testing.T) {
	svc, _ := testDB(t)

	cfg := &config.Config{}
	runner := NewRunner(cfg, nil, svc, nil)

	task := db.Task{ID: "task-noop", Goal: "test"}
	runner.RunCritic(t.Context(), task)
}

func TestRunCriticNoOpWhenNilLLM(t *testing.T) {
	svc, _ := testDB(t)

	cfg := &config.Config{
		Models: config.ModelsConfig{
			Critic: config.CriticConfig{Enabled: true},
		},
	}
	runner := NewRunner(cfg, nil, svc, nil)

	task := db.Task{ID: "task-noop-llm", Goal: "test"}
	runner.RunCritic(t.Context(), task)
}

func TestCancelReturnsFalseForUnknownTask(t *testing.T) {
	runner := &Runner{}
	if runner.Cancel("nonexistent") {
		t.Fatal("expected Cancel to return false for unknown task")
	}
}

func TestWaitBlocksUntilRunnersDone(t *testing.T) {
	runner := &Runner{}

	var done atomic.Bool

	runner.wg.Add(1)
	go func() {
		defer runner.wg.Done()
		time.Sleep(200 * time.Millisecond)
		done.Store(true)
	}()

	waited := make(chan struct{})
	go func() {
		runner.Wait()
		close(waited)
	}()

	select {
	case <-waited:
		if !done.Load() {
			t.Fatal("Wait returned before goroutine finished")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return within timeout")
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
