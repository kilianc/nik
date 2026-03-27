package task

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
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

func createStartedTask(t *testing.T, svc *Service, conn *sql.DB, goal string) db.Task {
	t.Helper()
	ctx := context.Background()

	task, err := svc.Create(ctx, createParams{
		Goal: goal, Thinking: "low", ConversationID: testConvID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	actID := "act-" + task.ID[:8]
	_, err = conn.ExecContext(ctx,
		"INSERT INTO activation (id, conversation_id, sources, model, created_at) VALUES (?, ?, '[]', 'test', NOW_ISO8601_MS())",
		actID, testConvID)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	err = svc.Start(ctx, task.ID, actID)
	if err != nil {
		t.Fatalf("start task: %v", err)
	}

	return task
}

func assertSystemMessage(t *testing.T, conn *sql.DB, kind, taskID, jsonPath, want string) {
	t.Helper()

	idPath := "$.task_id"
	if kind == "task_cancelled" || kind == "task_spawned" || kind == "task_retry" {
		idPath = "$.id"
	}

	var got string
	err := conn.QueryRowContext(context.Background(),
		fmt.Sprintf(
			`SELECT json_extract(body, '%s')
			 FROM message
			 WHERE platform = 'system'
			   AND kind = ?1
			   AND json_extract(body, '%s') = ?2
			 ORDER BY sent_at DESC LIMIT 1`, jsonPath, idPath),
		kind, taskID,
	).Scan(&got)
	if err != nil {
		t.Fatalf("query system message (kind=%s): %v", kind, err)
	}
	if got != want {
		t.Fatalf("system message %s = %q, want %q", jsonPath, got, want)
	}
}

func TestRunFinalization(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		svc, conn := testDB(t)
		ctx := context.Background()
		task := createStartedTask(t, svc, conn, "slow task")

		err := svc.Cancel(ctx, task.ID, "timed out")
		if err != nil {
			t.Fatalf("cancel: %v", err)
		}

		got, err := svc.Get(ctx, task.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status != "cancelled" {
			t.Fatalf("status = %q, want cancelled", got.Status)
		}

		assertSystemMessage(t, conn, "task_cancelled", task.ID, "$.cancellation_reason", "timed out")
	})

	t.Run("error", func(t *testing.T) {
		svc, conn := testDB(t)
		ctx := context.Background()
		task := createStartedTask(t, svc, conn, "broken task")

		errMsg := "Task terminated: max rounds (200) reached without completion"
		svc.InsertReport(ctx, task.ID, "failed", errMsg)
		svc.UpdateStatus(ctx, task.ID, "failed")

		got, err := svc.Get(ctx, task.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status != "failed" {
			t.Fatalf("status = %q, want failed", got.Status)
		}

		assertSystemMessage(t, conn, "task_report", task.ID, "$.status", "failed")
		assertSystemMessage(t, conn, "task_report", task.ID, "$.content", errMsg)
	})

	t.Run("clean exit without report", func(t *testing.T) {
		svc, conn := testDB(t)
		ctx := context.Background()
		task := createStartedTask(t, svc, conn, "quiet task")

		reportStatus, _ := svc.LastReportStatus(ctx, task.ID)
		finalStatus := "failed"
		if reportStatus == "completed" || reportStatus == "failed" {
			finalStatus = reportStatus
		}

		if finalStatus == "failed" {
			svc.InsertReport(ctx, task.ID, "failed", "Task ended without a completion report.")
		}
		svc.UpdateStatus(ctx, task.ID, finalStatus)

		got, err := svc.Get(ctx, task.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status != "failed" {
			t.Fatalf("status = %q, want failed", got.Status)
		}

		assertSystemMessage(t, conn, "task_report", task.ID, "$.content", "Task ended without a completion report.")
	})
}
