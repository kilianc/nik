package task

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/llm"
)

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

func TestFilterUnprivileged(t *testing.T) {
	handler := func(context.Context, llm.ToolCall) (string, error) { return "", nil }

	tests := []struct {
		name  string
		tools []llm.Tool
		want  int
	}{
		{
			"mixed",
			[]llm.Tool{
				{Def: llm.ToolDef{Name: "shell"}, Handler: handler, Privileged: true},
				{Def: llm.ToolDef{Name: "db_query"}, Handler: handler, Privileged: true},
				{Def: llm.ToolDef{Name: "describe_media"}, Handler: handler},
				{Def: llm.ToolDef{Name: "load_skill"}, Handler: handler},
			},
			2,
		},
		{
			"all public",
			[]llm.Tool{
				{Def: llm.ToolDef{Name: "describe_media"}, Handler: handler},
				{Def: llm.ToolDef{Name: "load_skill"}, Handler: handler},
			},
			2,
		},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterUnprivileged(tt.tools)
			if len(got) != tt.want {
				t.Fatalf("expected %d tools, got %d", tt.want, len(got))
			}
			for _, tool := range got {
				if tool.Privileged {
					t.Fatalf("privileged tool %q should have been filtered", tool.Def.Name)
				}
			}
		})
	}
}
