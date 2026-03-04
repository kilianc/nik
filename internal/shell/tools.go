package shell

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

var shellToolDef = llm.ToolDef{
	Name:        "shell",
	Description: "Run a shell command in a persistent tmux session.\n\nActions: run (start + watch), read (look at terminal), send (type + Enter and watch), kill (destroy).\n\nUse non-interactive flags (-y) when possible. For interactive prompts, use send.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"run", "read", "send", "kill"},
				"description": "run: start a command and watch. read: look at terminal output. send: type + Enter and watch. kill: destroy session.",
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
				"description": "Seconds to watch the terminal. Polls for completion -- returns early if the command finishes. Default 10.",
			},
			"watch_for": map[string]any{
				"type":        "string",
				"description": "String to watch for in terminal output. Returns early when found instead of waiting full max_wait.",
			},
		},
		"required":             []string{"action", "command", "description", "session_id", "input", "max_wait", "watch_for"},
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
	WatchFor    string `json:"watch_for"`
}

func BuildTools(cfg *config.Config) []llm.Tool {
	err := ensureTmux()
	if err != nil {
		slog.Warn("shell tool disabled", "pkg", "shell", "error", err)
		return nil
	}

	return []llm.Tool{
		{
			Def:        shellToolDef,
			Handler:    shellHandler(cfg.Home),
			Privileged: true,
		},
	}
}

func shellHandler(home string) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args shellArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		switch args.Action {
		case "run":
			return handleRun(ctx, args, home)
		case "read", "send":
			return handleInteract(ctx, args)
		case "kill":
			return handleKill(args)
		default:
			return llm.ToolErrorf("unknown action %q", args.Action), nil
		}
	}
}

func handleRun(ctx context.Context, args shellArgs, home string) (string, error) {
	if args.Command == "" {
		return `{"error":"empty command"}`, nil
	}

	sid := id.Short(4)

	err := newSession(sid, args.Command, home)
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
	)

	maxWait := args.MaxWait
	if maxWait == 0 {
		maxWait = 10
	}

	output, alive, code := stare(ctx, sid, maxWait, args.WatchFor)

	if !alive {
		killSession(sid)
		return llm.ToolResult(map[string]any{"status": "exited", "exit_code": code, "output": output}), nil
	}

	return llm.ToolResult(map[string]any{"status": "running", "session_id": sid, "output": output}), nil
}

func handleInteract(ctx context.Context, args shellArgs) (string, error) {
	if args.SessionID == "" {
		return `{"error":"empty session_id"}`, nil
	}

	// capture baseline before sending input so watch_for only matches new output
	var baseline int
	if args.WatchFor != "" {
		if out, err := capturePane(args.SessionID); err == nil {
			baseline = len(out)
		}
	}

	if args.Input != "" {
		err := sendKeys(args.SessionID, args.Input, "Enter")
		if err != nil {
			return llm.ToolError(err), nil
		}
	}

	output, alive, code := stareWith(ctx, args.SessionID, args.MaxWait, args.WatchFor, baseline)

	if !alive {
		killSession(args.SessionID)
		return llm.ToolResult(map[string]any{"status": "exited", "exit_code": code, "output": output}), nil
	}

	return llm.ToolResult(map[string]any{"status": "running", "session_id": args.SessionID, "output": output}), nil
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
