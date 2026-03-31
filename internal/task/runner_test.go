package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/prompt"
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

func openaiResponse(toolCalls ...string) string {
	var output []map[string]any
	for i := 0; i < len(toolCalls); i += 2 {
		output = append(output, map[string]any{
			"type":      "function_call",
			"id":        fmt.Sprintf("fc%d", i/2),
			"call_id":   fmt.Sprintf("c%d", i/2),
			"name":      toolCalls[i],
			"arguments": toolCalls[i+1],
			"status":    "completed",
		})
	}
	if len(toolCalls) == 0 {
		output = append(output, map[string]any{
			"type":   "message",
			"id":     "m1",
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]any{
				{"type": "output_text", "text": "done", "annotations": []any{}},
			},
		})
	}
	b, _ := json.Marshal(map[string]any{
		"id": "r1", "object": "response", "created_at": 0, "status": "completed",
		"output": output,
		"usage": map[string]any{
			"input_tokens": 10, "output_tokens": 5, "total_tokens": 15,
			"input_tokens_details":  map[string]any{"cached_tokens": 0},
			"output_tokens_details": map[string]any{"reasoning_tokens": 0},
		},
	})
	return string(b)
}

func setupRunnerTest(t *testing.T, responses []string) (*Runner, db.Task, *llm.Activation, llm.ToolExecutor) {
	t.Helper()

	svc, conn := testDB(t)
	ctx := context.Background()

	task := createStartedTask(t, svc, conn, "test task")

	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(reqCount.Add(1))
		w.Header().Set("Content-Type", "application/json")
		idx := n - 1
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		fmt.Fprint(w, responses[idx])
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()

	model := "test-model"
	client := llm.NewClient(&model, llm.WithAPIKey("test-key"), llm.WithBaseURL(srv.URL))

	cfg := &config.Config{Home: tmpDir}
	pr := prompt.NewRenderer(&config.Config{Home: tmpDir})
	runner := &Runner{cfg: cfg, pr: pr, svc: svc}

	reportTool := BuildReportTool(svc, task.ID)
	allTools := []llm.Tool{reportTool}
	defs, exec := llm.SplitTools(allTools)

	act := llm.NewActivation(client, llm.NoopRecorder{}, "test", defs)
	act.SetMaxRounds(20)
	act.Start(ctx)
	act.SetInput("")

	return runner, task, act, exec
}

func TestRunLoopDoneFlag(t *testing.T) {
	t.Run("exits after task_report completed", func(t *testing.T) {
		args, _ := json.Marshal(reportArgs{Status: "completed", Note: "all done"})
		responses := []string{
			openaiResponse("task_report", string(args)),
			openaiResponse(),
		}

		runner, task, act, exec := setupRunnerTest(t, responses)
		defer act.Close(context.Background())

		err := runner.runLoop(context.Background(), task, act, exec)
		if err != nil {
			t.Fatalf("runLoop returned error: %v", err)
		}

		status, err := runner.svc.LastReportStatus(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("LastReportStatus: %v", err)
		}
		if status != "completed" {
			t.Fatalf("last report status = %q, want completed", status)
		}

		if act.RoundNumber() != 2 {
			t.Fatalf("expected 2 rounds, got %d", act.RoundNumber())
		}
	})

	t.Run("exits after task_report failed", func(t *testing.T) {
		args, _ := json.Marshal(reportArgs{Status: "failed", Note: "could not finish"})
		responses := []string{
			openaiResponse("task_report", string(args)),
			openaiResponse(),
		}

		runner, task, act, exec := setupRunnerTest(t, responses)
		defer act.Close(context.Background())

		err := runner.runLoop(context.Background(), task, act, exec)
		if err != nil {
			t.Fatalf("runLoop returned error: %v", err)
		}

		status, err := runner.svc.LastReportStatus(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("LastReportStatus: %v", err)
		}
		if status != "failed" {
			t.Fatalf("last report status = %q, want failed", status)
		}

		if act.RoundNumber() != 2 {
			t.Fatalf("expected 2 rounds, got %d", act.RoundNumber())
		}
	})

	t.Run("continues after task_report running", func(t *testing.T) {
		args, _ := json.Marshal(reportArgs{Status: "running", Note: "still working"})
		responses := []string{
			openaiResponse("task_report", string(args)),
			openaiResponse(),
			openaiResponse(),
		}

		runner, task, act, exec := setupRunnerTest(t, responses)
		defer act.Close(context.Background())

		err := runner.runLoop(context.Background(), task, act, exec)
		if err != nil {
			t.Fatalf("runLoop returned error: %v", err)
		}

		status, err := runner.svc.LastReportStatus(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("LastReportStatus: %v", err)
		}
		if status != "running" {
			t.Fatalf("last report status = %q, want running", status)
		}

		if act.RoundNumber() < 3 {
			t.Fatalf("expected at least 3 rounds (nudge path), got %d", act.RoundNumber())
		}
	})
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
