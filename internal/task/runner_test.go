package task

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestReportTimerReset(t *testing.T) {
	if StaleThreshold != 2*time.Minute {
		t.Fatalf("StaleThreshold = %v, want 2m", StaleThreshold)
	}

	tests := []struct {
		name      string
		calls     []llm.ToolCall
		wantReset bool
	}{
		{
			"task_report resets timer",
			[]llm.ToolCall{{Name: "shell"}, {Name: "task_report"}, {Name: "write_file"}},
			true,
		},
		{
			"non-report calls do not reset",
			[]llm.ToolCall{{Name: "shell"}, {Name: "write_file"}, {Name: "load_skill"}},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastReport := time.Now().Add(-3 * time.Minute)

			for _, call := range tt.calls {
				if call.Name == "task_report" {
					lastReport = time.Now()
				}
			}

			stale := time.Since(lastReport) >= StaleThreshold
			if tt.wantReset && stale {
				t.Fatal("timer should have been reset by task_report")
			}
			if !tt.wantReset && !stale {
				t.Fatal("timer should not have been reset")
			}
		})
	}
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
