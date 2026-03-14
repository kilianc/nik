package task

import (
	"strings"
	"testing"

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
