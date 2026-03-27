package task

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

const (
	maxActivePerConversation = 5
	maxRetriesPerGoal        = 5
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
	Description: "Check on a task. Returns status, goal, plan, reports, and retry chain.",
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
			"reason": map[string]any{
				"type":        "string",
				"description": "Why the task is being cancelled.",
			},
		},
		"required":             []string{"task_id", "reason"},
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
	Reason string `json:"reason"`
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
			"thinking": map[string]any{
				"type":        "string",
				"enum":        []string{"low", "medium", "high"},
				"description": "Reasoning effort for the retry. Consider bumping if the previous attempt failed on a reasoning-heavy step. Empty keeps the original.",
			},
		},
		"required":             []string{"task_id", "goal", "plan", "thinking"},
		"additionalProperties": false,
	},
}

type retryArgs struct {
	TaskID   string `json:"task_id"`
	Goal     string `json:"goal"`
	Plan     string `json:"plan"`
	Thinking string `json:"thinking"`
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
			"plan":    t.Plan,
		}

		if t.Status == "cancelled" && t.CancellationReason != "" {
			result["cancellation_reason"] = t.CancellationReason
		}

		reports, _ := svc.ReportsByTask(ctx, t.ID)
		if len(reports) > 0 {
			formatted := make([]map[string]any, len(reports))
			for i, rpt := range reports {
				formatted[i] = map[string]any{
					"status":  rpt.Status,
					"content": rpt.Content,
					"at":      rpt.CreatedAt.Format("15:04:05"),
				}
			}
			result["reports"] = formatted
		}

		root := t.RetryForTaskID
		if root == "" {
			root = t.ID
		}

		chain, _ := svc.RetryChain(ctx, root)
		if len(chain) > 1 {
			formatted := make([]map[string]any, len(chain))
			for i, entry := range chain {
				e := map[string]any{
					"attempt": entry.RetryNumber,
					"status":  entry.Status,
					"goal":    entry.Goal,
				}
				if len(entry.Reports) > 0 {
					last := entry.Reports[len(entry.Reports)-1]
					e["last_report"] = last.Content
				}
				formatted[i] = e
			}
			result["retry_chain"] = formatted
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

		if args.Reason == "" {
			return llm.ToolErrorf("reason is required"), nil
		}

		taskID, err := svc.ResolveTaskID(ctx, args.TaskID)
		if err != nil {
			return llm.ToolError(err), nil
		}

		runner.Cancel(taskID)

		err = svc.Cancel(ctx, taskID, args.Reason)
		if err != nil {
			return llm.ToolError(err), nil
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

			err = svc.Cancel(ctx, original.ID, "superseded by retry")
			if err != nil {
				return llm.ToolError(err), nil
			}
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

		thinking := args.Thinking
		if thinking == "" {
			thinking = original.Thinking
		}

		t, err := svc.Create(ctx, createParams{
			ConversationID: original.ConversationID,
			ContactID:      original.ContactID,
			RetryForTaskID: root,
			RetryNumber:    retryNumber,
			Goal:           goal,
			Plan:           args.Plan,
			Thinking:       thinking,
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
