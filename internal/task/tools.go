package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/crew"
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
			"member": map[string]any{
				"type":        "string",
				"description": "Name of the crew member to assign this to. Omit to run unassigned.",
			},
		},
		"required":             []string{"goal", "plan", "thinking", "member"},
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
			"note": map[string]any{
				"type":        "string",
				"description": "What to tell your manager: progress, a blocker, or the final result.",
			},
		},
		"required":             []string{"note"},
		"additionalProperties": false,
	},
}

type spawnArgs struct {
	Goal     string `json:"goal"`
	Plan     string `json:"plan"`
	Thinking string `json:"thinking"`
	Member   string `json:"member"`
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
	Note string `json:"note"`
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
			"member": map[string]any{
				"type":        "string",
				"description": "Crew member. Empty = same as original.",
			},
		},
		"required":             []string{"task_id", "goal", "plan", "member"},
		"additionalProperties": false,
	},
}

type retryArgs struct {
	TaskID string `json:"task_id"`
	Goal   string `json:"goal"`
	Plan   string `json:"plan"`
	Member string `json:"member"`
}

func BuildTools(svc *Service, runner *Runner, crewSvc *crew.Service) []llm.Tool {
	return []llm.Tool{
		{Def: spawnToolDef, Handler: spawnHandler(svc, runner, crewSvc)},
		{Def: retryToolDef, Handler: retryHandler(svc, runner, crewSvc)},
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

func spawnHandler(svc *Service, runner *Runner, crewSvc *crew.Service) llm.ToolExecutor {
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

		var member *crew.Member
		var crewMemberID string

		if args.Member != "" {
			m, memberErr := crewSvc.Get(ctx, args.Member)
			if memberErr != nil {
				return llm.ToolErrorf("crew member %q not found: %v", args.Member, memberErr), nil
			}

			member = &m
			crewMemberID = m.ID
		}

		brainMeta, _ := ctx.Value("meta").(map[string]string)

		taskMeta := map[string]string{}
		if v := brainMeta["conversation_id"]; v != "" {
			taskMeta["conversation_id"] = v
		}
		if v := brainMeta["contact_id"]; v != "" {
			taskMeta["contact_id"] = v
		}

		convID := taskMeta["conversation_id"]
		if convID != "" {
			active, _ := svc.ActiveTasks(ctx, convID)
			for _, a := range active {
				if a.Goal == args.Goal {
					return llm.ToolErrorf("task %s already running with goal %q -- cancel it first or check its status", a.ID, a.Goal), nil
				}
			}
			if len(active) >= maxActivePerConversation {
				return llm.ToolErrorf("already %d active tasks for this conversation (max %d) -- wait for some to finish or cancel unneeded ones", len(active), maxActivePerConversation), nil
			}
		}

		t, err := svc.Create(ctx, CreateParams{
			CrewMemberID: crewMemberID,
			Goal:         args.Goal,
			Plan:         args.Plan,
			Thinking:     args.Thinking,
			Meta:         taskMeta,
		})
		if err != nil {
			return llm.ToolError(err), nil
		}

		go runner.Run(context.Background(), t, member)

		result := map[string]any{
			"task_id": t.ID,
			"status":  "pending",
			"goal":    t.Goal,
		}
		if member != nil {
			result["assigned_to"] = member.Name
		}

		return llm.ToolResult(result), nil
	}
}

func listHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args listArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		tasks, err := svc.List(ctx, args.IncludeRecent)
		if err != nil {
			return llm.ToolError(err), nil
		}

		if len(tasks) == 0 {
			return llm.ToolResult(map[string]any{"tasks": []any{}, "count": 0}), nil
		}

		items := make([]map[string]any, len(tasks))
		for i, t := range tasks {
			item := map[string]any{
				"task_id":    t.ID,
				"goal":       t.Goal,
				"status":     t.Status,
				"created_at": t.CreatedAt.Format("2006-01-02 15:04:05"),
			}

			if t.ConversationID.Valid {
				item["conversation_id"] = t.ConversationID.String
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

		t, err := svc.Get(ctx, args.TaskID)
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

		cancelled := runner.Cancel(args.TaskID)

		if !cancelled {
			err = svc.UpdateStatus(ctx, args.TaskID, "cancelled")
			if err != nil {
				return llm.ToolError(err), nil
			}
		}

		return llm.ToolResult(map[string]any{
			"task_id":   args.TaskID,
			"cancelled": true,
		}), nil
	}
}

func retryHandler(svc *Service, runner *Runner, crewSvc *crew.Service) llm.ToolExecutor {
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

		original, err := svc.Get(ctx, args.TaskID)
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

		brainMeta, _ := ctx.Value("meta").(map[string]string)
		convID := brainMeta["conversation_id"]
		if convID != "" {
			active, _ := svc.ActiveTasks(ctx, convID)
			if len(active) >= maxActivePerConversation {
				return llm.ToolErrorf("already %d active tasks (max %d) -- wait for some to finish", len(active), maxActivePerConversation), nil
			}
		}

		goal := args.Goal
		if goal == "" {
			goal = original.Goal
		}

		var member *crew.Member
		crewMemberID := original.CrewMemberID

		if args.Member != "" {
			m, memberErr := crewSvc.Get(ctx, args.Member)
			if memberErr != nil {
				return llm.ToolErrorf("crew member %q not found: %v", args.Member, memberErr), nil
			}
			member = &m
			crewMemberID = m.ID
		} else if crewMemberID != "" {
			m, memberErr := crewSvc.Get(ctx, crewMemberID)
			if memberErr == nil {
				member = &m
			}
		}

		chain, _ := svc.RetryChain(ctx, root)
		plan := buildRetryPlan(args.Plan, chain)

		t, err := svc.Create(ctx, CreateParams{
			CrewMemberID:   crewMemberID,
			RetryForTaskID: root,
			RetryNumber:    retryNumber,
			Goal:           goal,
			Plan:           plan,
			Thinking:       original.Thinking,
			Meta:           original.Meta,
		})
		if err != nil {
			return llm.ToolError(err), nil
		}

		go runner.Run(context.Background(), t, member)

		result := map[string]any{
			"task_id":      t.ID,
			"status":       "pending",
			"goal":         t.Goal,
			"retry_number": retryNumber,
		}
		if member != nil {
			result["assigned_to"] = member.Name
		}

		return llm.ToolResult(result), nil
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

		err = svc.InsertReport(ctx, taskID, args.Note)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return llm.ToolResult(map[string]any{"ok": true}), nil
	}
}
