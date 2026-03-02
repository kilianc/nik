package shell

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

const defaultCheckIn = 5 * time.Minute

var shellToolDef = llm.ToolDef{
	Name:        "shell",
	Description: "Your personal shell. Each run opens a new tmux session -- just pass the raw command. Never wrap commands in tmux/screen/nohup/bg yourself, and never ask the user how to run things.\n\nTwo modes:\n- Staring (max_wait): watch output live, returns early on exit. Costs a round.\n- Checking in (next_check_at): schedule a reminder and yield. You get the output later.\n\nOnce next_check_at is set, you're done with that session this turn. Reply to the user and stop. The reminder fires automatically -- do not read the session again.\n\nActions: run (start + watch), read (look / stare), send (type + watch), kill (destroy).\n\nUse non-interactive flags (-y) when possible. For interactive prompts, use send.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"run", "read", "send", "kill"},
				"description": "run: start a command and watch. read: look at terminal (or stare with max_wait). send: type + Enter and watch. kill: destroy session.",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command (run only). Empty for other actions.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Note: what this does and who asked. Example: 'Database backup for Kevin'. (run only). Empty for other actions.",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "Target session (read/send/kill). Empty for run.",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "Text to type + Enter (send only). Empty for other actions.",
			},
			"max_wait": map[string]any{
				"type":        "integer",
				"description": "Seconds to watch the terminal. Polls for completion -- returns early if the command finishes. Default 10 for run, 0 for read/send. Use on read/send to 'stare' at the screen after looking or typing.",
			},
			"next_check_at": map[string]any{
				"type":        "string",
				"description": "When to come back and check (RFC3339 absolute timestamp). You receive the session output at this time and decide what to do. Required for run -- estimate based on expected duration. Optional for read/send -- omit to keep the current schedule.",
			},
		},
		"required":             []string{"action", "command", "description", "session_id", "input", "max_wait", "next_check_at"},
		"additionalProperties": false,
	},
}

type shellArgs struct {
	Action      string `json:"action"`
	Command     string `json:"command"`
	Description string `json:"description"`
	SessionID   string `json:"session_id"`
	Input       string `json:"input"`
	MaxWait     int    `json:"max_wait"`
	NextCheckAt string `json:"next_check_at"`
}

func BuildTools() []llm.Tool {
	err := ensureTmux()
	if err != nil {
		slog.Warn("shell tool disabled", "pkg", "shell", "error", err)
		return nil
	}

	return []llm.Tool{
		{
			Def:        shellToolDef,
			Handler:    shellHandler(),
			Privileged: true,
		},
	}
}

func shellHandler() llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args shellArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		switch args.Action {
		case "run":
			return handleRun(ctx, args)
		case "read", "send":
			return handleInteract(args)
		case "kill":
			return handleKill(args)
		default:
			return llm.ToolErrorf("unknown action %q", args.Action), nil
		}
	}
}

func handleRun(ctx context.Context, args shellArgs) (string, error) {
	if args.Command == "" {
		return `{"error":"empty command"}`, nil
	}
	if args.NextCheckAt == "" {
		return `{"error":"next_check_at required for run"}`, nil
	}

	nextCheckAt, err := time.Parse(time.RFC3339, args.NextCheckAt)
	if err != nil {
		return llm.ToolErrorf("parse next_check_at %q: %s", args.NextCheckAt, err), nil
	}

	sid := id.Short(4)

	err = newSession(sid, args.Command)
	if err != nil {
		return llm.ToolError(err), nil
	}

	ctxMeta, _ := ctx.Value("meta").(map[string]string)
	now := time.Now().UTC()

	meta := SessionMeta{
		Command:        args.Command,
		Description:    args.Description,
		ConversationID: ctxMeta["conversation_id"],
		MessageID:      ctxMeta["message_id"],
		ActivationID:   ctxMeta["activation_id"],
		NextCheckAt:    nextCheckAt.UTC(),
		StartedAt:      now,
	}

	err = saveMeta(sid, meta)
	if err != nil {
		killSession(sid)
		return llm.ToolError(err), nil
	}

	slog.Info("shell run", "pkg", "shell", "id", sid,
		"command", args.Command,
		"description", args.Description,
		"conversation_id", meta.ConversationID,
		"message_id", meta.MessageID,
		"next_check_at", meta.NextCheckAt.Format(time.RFC3339),
	)

	maxWait := args.MaxWait
	if maxWait == 0 {
		maxWait = 10
	}

	output, alive, code := stare(sid, maxWait)

	if !alive {
		killSession(sid)
		return llm.ToolResult(map[string]any{"status": "exited", "exit_code": code, "output": output}), nil
	}

	nca := meta.NextCheckAt.Format(time.RFC3339)
	return llm.ToolResult(map[string]any{"status": "running", "session_id": sid, "next_check_at": nca, "output": output}), nil
}

func handleInteract(args shellArgs) (string, error) {
	if args.SessionID == "" {
		return `{"error":"empty session_id"}`, nil
	}

	if args.Input != "" {
		err := sendKeys(args.SessionID, args.Input, "Enter")
		if err != nil {
			return llm.ToolError(err), nil
		}
	}

	output, alive, code := stare(args.SessionID, args.MaxWait)

	if !alive {
		killSession(args.SessionID)
		return llm.ToolResult(map[string]any{"status": "exited", "exit_code": code, "output": output}), nil
	}

	meta, _ := loadMeta(args.SessionID)

	if args.NextCheckAt != "" {
		nextCheckAt, err := time.Parse(time.RFC3339, args.NextCheckAt)
		if err == nil {
			meta.NextCheckAt = nextCheckAt.UTC()
			saveMeta(args.SessionID, meta)
		}
	}

	nca := meta.NextCheckAt.Format(time.RFC3339)
	return llm.ToolResult(map[string]any{"status": "running", "next_check_at": nca, "output": output}), nil
}

func handleKill(args shellArgs) (string, error) {
	if args.SessionID == "" {
		return `{"error":"empty session_id"}`, nil
	}

	err := killSession(args.SessionID)
	if err != nil {
		return llm.ToolError(err), nil
	}

	return `{"ok":true}`, nil
}
