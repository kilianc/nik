package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/kciuffolo/nik/internal/crew"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/queries"
)

var spawnToolDef = llm.ToolDef{
	Name:        "task_spawn",
	Description: "Delegate heavy or long-running work to a background task. You write the plan, the task executes it. Use for anything that takes more than a few seconds (builds, research, multi-step ops). Returns the task ID.",
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
	Description: "Report progress or flag a blocker to your manager.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"note": map[string]any{
				"type":        "string",
				"description": "What happened or what you need.",
			},
			"needs_attention": map[string]any{
				"type":        "boolean",
				"description": "True if you need your manager to check in.",
			},
		},
		"required":             []string{"note", "needs_attention"},
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

type cancelArgs struct {
	TaskID string `json:"task_id"`
}

type reportArgs struct {
	Note           string `json:"note"`
	NeedsAttention bool   `json:"needs_attention"`
}

func BuildTools(svc *Service, runner *Runner, crewSvc *crew.Service) []llm.Tool {
	return []llm.Tool{
		{Def: spawnToolDef, Handler: spawnHandler(svc, runner, crewSvc)},
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

		meta, _ := ctx.Value("meta").(map[string]string)
		source := meta["source"]
		sourceID := meta["conversation_id"]
		if sourceID == "" {
			sourceID = meta["source_id"]
		}

		t, err := svc.Create(ctx, source, sourceID, crewMemberID, args.Goal, args.Plan, args.Thinking)
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
			toolCalls, tcErr := queryToolCalls(ctx, svc.db, t.ActivationID)
			if tcErr == nil && len(toolCalls) > 0 {
				result["recent_tool_calls"] = toolCalls
			}
		}

		return llm.ToolResult(result), nil
	}
}

type toolCallInfo struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
	Error      bool   `json:"error"`
	At         string `json:"at"`
}

func queryToolCalls(ctx context.Context, conn *sql.DB, activationID string) ([]toolCallInfo, error) {
	rows, err := conn.QueryContext(ctx, queries.TaskToolCalls, activationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []toolCallInfo
	for rows.Next() {
		var tc toolCallInfo
		var errFlag int
		var createdAt time.Time

		err = rows.Scan(
			&tc.Name,
			&tc.DurationMS,
			&errFlag,
			&createdAt,
		)
		if err != nil {
			return nil, err
		}

		tc.Error = errFlag != 0
		tc.At = createdAt.Format("15:04:05")
		calls = append(calls, tc)
	}

	return calls, rows.Err()
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

func reportHandler(svc *Service, taskID string) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args reportArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		kind := "attention"
		if !args.NeedsAttention {
			kind = "result"
		}

		err = svc.InsertReport(ctx, taskID, kind, args.Note)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return llm.ToolResult(map[string]any{"ok": true}), nil
	}
}
