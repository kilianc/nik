package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

const (
	maxActivePerConversation = 5
	maxRetriesPerGoal        = 3
)

var spawnToolDef = llm.ToolDef{
	Name:        "task_spawn",
	Description: "Start a new background task. For retrying failed work, use task_retry instead.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"contact_id": map[string]any{
				"type":        "string",
				"description": "Canonical contact_id for who requested the task.",
			},
			"goal": map[string]any{
				"type":        "string",
				"description": "Short label for the task (shown in status).",
			},
			"plan": map[string]any{
				"type":        "string",
				"description": "Detailed instructions: steps, what to check, what to report. The task executes your plan.",
			},
			"thinking": map[string]any{
				"type":        "string",
				"enum":        []string{"low", "medium", "high"},
				"description": "Reasoning effort for the task. low for simple commands, high for complex research.",
			},
		},
		"required":             []string{"contact_id", "goal", "plan", "thinking"},
		"additionalProperties": false,
	},
}

var statusToolDef = llm.ToolDef{
	Name:        "task_status",
	Description: "Check on a running task. Returns status, goal, duration, and recent tool calls.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "ID of the task to check.",
			},
		},
		"required":             []string{"task_id"},
		"additionalProperties": false,
	},
}

var listToolDef = llm.ToolDef{
	Name:        "task_list",
	Description: "List active tasks and optionally recently finished ones.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"include_recent": map[string]any{
				"type":        "boolean",
				"description": "When true, also includes tasks completed/failed/cancelled in the last hour. Omit or false for active only.",
			},
		},
		"required":             []string{"include_recent"},
		"additionalProperties": false,
	},
}

var cancelToolDef = llm.ToolDef{
	Name:        "task_cancel",
	Description: "Cancel a running task.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "ID of the task to cancel.",
			},
		},
		"required":             []string{"task_id"},
		"additionalProperties": false,
	},
}

var reportToolDef = llm.ToolDef{
	Name:        "task_report",
	Description: "Send an update to your manager. Use for progress, blockers, or your final result. Your manager only sees what you report -- if you don't report, they don't know.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"running", "completed", "failed"},
				"description": "running = progress update, completed = task done successfully, failed = task cannot be completed.",
			},
			"note": map[string]any{
				"type":        "string",
				"description": "What to tell your manager: progress, a blocker, or the final result.",
			},
		},
		"required":             []string{"status", "note"},
		"additionalProperties": false,
	},
}

type spawnArgs struct {
	ContactID string `json:"contact_id"`
	Goal      string `json:"goal"`
	Plan      string `json:"plan"`
	Thinking  string `json:"thinking"`
}

type statusArgs struct {
	TaskID string `json:"task_id"`
}

type listArgs struct {
	IncludeRecent bool `json:"include_recent"`
}

type cancelArgs struct {
	TaskID string `json:"task_id"`
}

type reportArgs struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

var retryToolDef = llm.ToolDef{
	Name:        "task_retry",
	Description: "Retry a failed task with a better plan. Use instead of task_spawn for work that already failed.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "ID of the failed task to retry.",
			},
			"goal": map[string]any{
				"type":        "string",
				"description": "Updated goal. Empty keeps the original.",
			},
			"plan": map[string]any{
				"type":        "string",
				"description": "New plan -- what to do differently.",
			},
		},
		"required":             []string{"task_id", "goal", "plan"},
		"additionalProperties": false,
	},
}

type retryArgs struct {
	TaskID string `json:"task_id"`
	Goal   string `json:"goal"`
	Plan   string `json:"plan"`
}

func BuildTools(svc *Service, runner *Runner) []llm.Tool {
	return []llm.Tool{
		{Def: spawnToolDef, Handler: spawnHandler(svc, runner)},
		{Def: retryToolDef, Handler: retryHandler(svc, runner)},
		{Def: listToolDef, Handler: listHandler(svc)},
		{Def: statusToolDef, Handler: statusHandler(svc)},
		{Def: cancelToolDef, Handler: cancelHandler(svc, runner)},
	}
}

func BuildReportTool(svc *Service, taskID string) llm.Tool {
	return llm.Tool{
		Def:     reportToolDef,
		Handler: reportHandler(svc, taskID),
	}
}

func spawnHandler(svc *Service, runner *Runner) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args spawnArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if args.Goal == "" {
			return llm.ToolErrorf("goal is required"), nil
		}
		if args.Plan == "" {
			return llm.ToolErrorf("plan is required"), nil
		}

		brainMeta, _ := ctx.Value("meta").(map[string]string)

		convID := brainMeta["conversation_id"]

		if convID != "" {
			active, _ := svc.ListTasks(ctx, db.TaskListParams{ConversationID: convID})
			for _, a := range active {
				if a.Goal == args.Goal {
					return llm.ToolErrorf("task %s already running with goal %q -- cancel it first or check its status", a.ID, a.Goal), nil
				}
			}
			if len(active) >= maxActivePerConversation {
				return llm.ToolErrorf("already %d active tasks for this conversation (max %d) -- wait for some to finish or cancel unneeded ones", len(active), maxActivePerConversation), nil
			}
		}

		t, err := svc.Create(ctx, createParams{
			ConversationID: convID,
			ContactID:      args.ContactID,
			Goal:           args.Goal,
			Plan:           args.Plan,
			Thinking:       args.Thinking,
		})
		if err != nil {
			return llm.ToolError(err), nil
		}

		runner.wg.Add(1)
		go runner.Run(context.Background(), t)

		return llm.ToolResult(map[string]any{
			"task_id": t.ID,
			"status":  "pending",
			"goal":    t.Goal,
		}), nil
	}
}

func listHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args listArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		tasks, err := svc.ListTasks(ctx, db.TaskListParams{IncludeRecent: args.IncludeRecent})
		if err != nil {
			return llm.ToolError(err), nil
		}

		if len(tasks) == 0 {
			return llm.ToolResult(map[string]any{"tasks": []any{}, "count": 0}), nil
		}

		items := make([]map[string]any, len(tasks))
		for i, t := range tasks {
			item := map[string]any{
				"task_id":         t.ID,
				"goal":            t.Goal,
				"status":          t.Status,
				"conversation_id": t.ConversationID,
				"created_at":      t.CreatedAt.Format("2006-01-02 15:04:05"),
			}

			if t.StartedAt.Valid {
				if t.CompletedAt.Valid {
					item["duration"] = t.CompletedAt.Time.Sub(t.StartedAt.Time).Truncate(time.Second).String()
				} else {
					item["duration"] = time.Since(t.StartedAt.Time).Truncate(time.Second).String()
				}
			}

			items[i] = item
		}

		return llm.ToolResult(map[string]any{"tasks": items, "count": len(items)}), nil
	}
}

func statusHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args statusArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		taskID, err := svc.ResolveTaskID(ctx, args.TaskID)
		if err != nil {
			return llm.ToolError(err), nil
		}

		t, err := svc.Get(ctx, taskID)
		if err != nil {
			return llm.ToolError(err), nil
		}

		result := map[string]any{
			"task_id": t.ID,
			"status":  t.Status,
			"goal":    t.Goal,
		}

		if t.StartedAt.Valid {
			if t.CompletedAt.Valid {
				result["duration"] = t.CompletedAt.Time.Sub(t.StartedAt.Time).Truncate(time.Second).String()
			} else {
				result["duration"] = time.Since(t.StartedAt.Time).Truncate(time.Second).String()
			}
		}

		if t.ActivationID != "" {
			toolCalls, tcErr := svc.RecentToolCalls(ctx, t.ActivationID)
			if tcErr == nil && len(toolCalls) > 0 {
				formatted := make([]map[string]any, len(toolCalls))
				for i, tc := range toolCalls {
					formatted[i] = map[string]any{
						"name":        tc.Name,
						"round":       tc.Round,
						"input":       tc.Input,
						"output":      tc.Output,
						"duration_ms": tc.DurationMS,
						"error":       tc.Error,
						"at":          tc.At.Format("15:04:05"),
					}
				}
				result["recent_tool_calls"] = formatted
			}
		}

		return llm.ToolResult(result), nil
	}
}

func cancelHandler(svc *Service, runner *Runner) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args cancelArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		taskID, err := svc.ResolveTaskID(ctx, args.TaskID)
		if err != nil {
			return llm.ToolError(err), nil
		}

		cancelled := runner.Cancel(taskID)

		if !cancelled {
			err = svc.UpdateStatus(ctx, taskID, "cancelled")
			if err != nil {
				return llm.ToolError(err), nil
			}
		}

		return llm.ToolResult(map[string]any{
			"task_id":   taskID,
			"cancelled": true,
		}), nil
	}
}

func retryHandler(svc *Service, runner *Runner) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args retryArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if args.TaskID == "" {
			return llm.ToolErrorf("task_id is required"), nil
		}
		if args.Plan == "" {
			return llm.ToolErrorf("plan is required"), nil
		}

		taskID, err := svc.ResolveTaskID(ctx, args.TaskID)
		if err != nil {
			return llm.ToolError(err), nil
		}

		original, err := svc.Get(ctx, taskID)
		if err != nil {
			return llm.ToolErrorf("task %s not found: %v", args.TaskID, err), nil
		}

		if original.Status == "pending" || original.Status == "running" {
			runner.Cancel(original.ID)
			svc.UpdateStatus(ctx, original.ID, "cancelled")
		}

		root := original.RetryForTaskID
		if root == "" {
			root = original.ID
		}

		retryNumber := original.RetryNumber + 1
		if retryNumber > maxRetriesPerGoal {
			return llm.ToolErrorf("retry limit reached (%d attempts) for this task chain -- tell the user what's wrong instead", maxRetriesPerGoal), nil
		}

		activeRetries, _ := svc.ActiveRetries(ctx, root)
		if len(activeRetries) > 0 {
			return llm.ToolErrorf("there is already an active task in this retry chain (%s) -- cancel it first", activeRetries[0].ID), nil
		}

		convID := original.ConversationID
		if convID != "" {
			active, _ := svc.ListTasks(ctx, db.TaskListParams{ConversationID: convID})
			if len(active) >= maxActivePerConversation {
				return llm.ToolErrorf("already %d active tasks (max %d) -- wait for some to finish", len(active), maxActivePerConversation), nil
			}
		}

		goal := args.Goal
		if goal == "" {
			goal = original.Goal
		}

		chain, _ := svc.RetryChain(ctx, root)
		plan := buildRetryPlan(args.Plan, chain)

		t, err := svc.Create(ctx, createParams{
			ConversationID: original.ConversationID,
			ContactID:      original.ContactID,
			RetryForTaskID: root,
			RetryNumber:    retryNumber,
			Goal:           goal,
			Plan:           plan,
			Thinking:       original.Thinking,
		})
		if err != nil {
			return llm.ToolError(err), nil
		}

		runner.wg.Add(1)
		go runner.Run(context.Background(), t)

		return llm.ToolResult(map[string]any{
			"task_id":      t.ID,
			"status":       "pending",
			"goal":         t.Goal,
			"retry_number": retryNumber,
		}), nil
	}
}

func buildRetryPlan(newPlan string, chain []db.RetryChainEntry) string {
	if len(chain) == 0 {
		return newPlan
	}

	var b strings.Builder
	b.WriteString("## Previous attempts\n\n")
	for _, entry := range chain {
		fmt.Fprintf(&b, "### Attempt #%d (%s)\n", entry.RetryNumber, entry.Status)
		fmt.Fprintf(&b, "Goal: %s\n", entry.Goal)
		for _, r := range entry.Reports {
			fmt.Fprintf(&b, "Report: %s\n", r.Content)
		}
		b.WriteByte('\n')
	}
	b.WriteString("---\n\n## New plan\n\n")
	b.WriteString(newPlan)

	return b.String()
}

func reportHandler(svc *Service, taskID string) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args reportArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		err = svc.InsertReport(ctx, taskID, args.Status, args.Note)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return llm.ToolResult(map[string]any{"ok": true}), nil
	}
}
