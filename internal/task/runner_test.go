package task

import (
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
