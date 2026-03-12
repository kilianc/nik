package shell

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

var shellToolDef = llm.ToolDef{
	Name:        "shell",
	Description: "Run a shell command in a persistent tmux session.\n\nActions: run (start + watch), read (look at terminal), send (type + Enter and watch), kill (destroy).\n\nReturns early if the command finishes before max_wait. For long-running processes, use a short max_wait then check back with read.\n\nUse non-interactive flags (-y) when possible. For interactive prompts, use send.",
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
				"description": "Seconds to watch the terminal. Returns early if the command finishes. Default 10.",
			},
		},
		"required":             []string{"action", "command", "description", "session_id", "input", "max_wait"},
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
}

func (s *Service) BuildTools() []llm.Tool {
	err := ensureTmux()
	if err != nil {
		slog.Warn("shell tool disabled", "pkg", "shell", "error", err)
		return nil
	}

	return []llm.Tool{
		{
			Def:        shellToolDef,
			Handler:    s.shellHandler(),
			Privileged: true,
		},
	}
}

func (s *Service) shellHandler() llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args shellArgs

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		switch args.Action {
		case "run":
			return s.handleRun(ctx, args)
		case "read", "send":
			return s.handleInteract(ctx, args)
		case "kill":
			return handleKill(args)
		default:
			return llm.ToolErrorf("unknown action %q", args.Action), nil
		}
	}
}

func (s *Service) handleRun(ctx context.Context, args shellArgs) (string, error) {
	if args.Command == "" {
		return `{"error":"empty command"}`, nil
	}

	sid := id.Short(4)

	err := newSession(sid, args.Command, s.home)
	if err != nil {
		return llm.ToolError(err), nil
	}

	ctxMeta, _ := ctx.Value("meta").(map[string]string)
	now := time.Now().UTC()

	meta := SessionMeta{
		Command:        args.Command,
		Description:    args.Description,
		ConversationID: ctxMeta["conversation_id"],
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

	output, alive, code := stare(ctx, sid, maxWait)

	s.persistOutput(ctx, sid, args.Command, args.Description, output, alive, code)

	if !alive {
		killSession(sid)
		return shellResult(sid, output, alive, code), nil
	}

	return shellResult(sid, output, alive, code), nil
}

func (s *Service) handleInteract(ctx context.Context, args shellArgs) (string, error) {
	if args.SessionID == "" {
		return `{"error":"empty session_id"}`, nil
	}

	if args.Input != "" {
		err := sendKeys(args.SessionID, args.Input, "Enter")
		if err != nil {
			return llm.ToolError(err), nil
		}
	}

	if !isAlive(args.SessionID) {
		out, _ := capturePane(args.SessionID)
		code, _ := getExitCode(args.SessionID)
		killSession(args.SessionID)
		s.persistOutput(ctx, args.SessionID, "", "", out, false, code)
		return shellResult(args.SessionID, out, false, code), nil
	}

	maxWait := args.MaxWait
	if maxWait == 0 {
		maxWait = 10
	}

	output, alive, code := stare(ctx, args.SessionID, maxWait)

	s.persistOutput(ctx, args.SessionID, "", "", output, alive, code)

	if !alive {
		killSession(args.SessionID)
	}

	return shellResult(args.SessionID, output, alive, code), nil
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

func (s *Service) persistOutput(ctx context.Context, sid, command, description, output string, alive bool, exitCode int) {
	if s.conn == nil {
		return
	}

	var codePtr *int
	if !alive {
		codePtr = &exitCode
	}

	err := db.ShellOutputUpsert(ctx, s.conn, db.ShellOutputUpsertParams{
		SessionID:   sid,
		Command:     command,
		Description: description,
		Output:      output,
		ExitCode:    codePtr,
		Alive:       alive,
	})
	if err != nil {
		slog.Warn("persist shell output", "pkg", "shell", "session_id", sid, "error", err)
	}
}

func shellResult(sid, output string, alive bool, exitCode int) string {
	truncated := len(output) > maxContextBytes
	contextOutput := output
	if truncated {
		contextOutput = output[len(output)-maxContextBytes:]
	}

	result := map[string]any{
		"output": contextOutput,
	}

	if truncated {
		result["truncated"] = true
		result["total_bytes"] = len(output)
	}

	if alive {
		result["status"] = "running"
		result["session_id"] = sid
	} else {
		result["status"] = "exited"
		result["exit_code"] = exitCode
	}

	return llm.ToolResult(result)
}
