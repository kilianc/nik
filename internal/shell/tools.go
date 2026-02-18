package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

const defaultCheckIn = 5 * time.Minute

var shellToolDef = llm.ToolDef{
	Name:        "shell",
	Description: "Your personal shell. Each run opens a new tmux session -- just pass the raw command. Never wrap commands in tmux/screen/nohup/bg yourself, and never ask the user how to run things.\n\nTwo modes:\n- Staring (max_wait): watch output live, returns early on exit. Costs a round.\n- Checking in (next_check_at): schedule a reminder and yield. You get the output later.\n\nOnce next_check_at is set, you're done with that session this turn. Reply to the user and stop. The reminder fires automatically -- do not read the session again.\n\nActions: run (start + watch), read (look / stare), send (type + watch), kill (destroy), list (show all).\n\nUse non-interactive flags (-y) when possible. For interactive prompts, use send.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"run", "read", "send", "kill", "list"},
				"description": "run: start a command and watch. read: look at terminal (or stare with max_wait). send: type + Enter and watch. kill: destroy session. list: show all sessions.",
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
				"description": "Target session (read/send/kill). Empty for run/list.",
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
				"description": "When to come back and check (RFC3339 or relative: '+30s', '+5m', '+1h', '+1d'). You receive the session output at this time and decide what to do. Required for run -- estimate based on expected duration: +30s for prompts, +5m for builds, +1d for services. Optional for read/send -- omit to keep the current schedule.",
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
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		switch args.Action {
		case "run":
			return handleRun(ctx, args)
		case "read":
			return handleRead(args)
		case "send":
			return handleSend(args)
		case "kill":
			return handleKill(args)
		case "list":
			return handleList()
		default:
			return fmt.Sprintf(`{"error":"unknown action %q"}`, args.Action), nil
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

	checkAt, err := parseNextCheckAt(args.NextCheckAt)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	id := db.NewID()[:8]

	err = newSession(id, args.Command)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	meta, _ := ctx.Value("meta").(map[string]string)
	conversationID := meta["conversation_id"]
	messageID := meta["message_id"]

	setEnv(id, "NIK_COMMAND", args.Command)
	setEnv(id, "NIK_DESCRIPTION", args.Description)
	setEnv(id, "NIK_CONVERSATION_ID", conversationID)
	setEnv(id, "NIK_MESSAGE_ID", messageID)
	setEnv(id, "NIK_STARTED_AT", time.Now().UTC().Format(time.RFC3339))
	setEnv(id, "NIK_NEXT_CHECK_AT", checkAt.UTC().Format(time.RFC3339))

	slog.Info("shell run", "pkg", "shell", "id", id,
		"command", args.Command,
		"description", args.Description,
		"conversation_id", conversationID,
		"message_id", messageID,
		"next_check_at", checkAt.UTC().Format(time.RFC3339),
	)

	maxWait := args.MaxWait
	if maxWait == 0 {
		maxWait = 10
	}

	output, alive, code := stare(id, maxWait)

	if !alive {
		killSession(id)
		return fmt.Sprintf(`{"status":"exited","exit_code":%d,"output":%q}`, code, output), nil
	}

	nca := checkAt.UTC().Format(time.RFC3339)
	return fmt.Sprintf(`{"status":"running","session_id":%q,"next_check_at":%q,"output":%q}`, id, nca, output), nil
}

func handleRead(args shellArgs) (string, error) {
	if args.SessionID == "" {
		return `{"error":"empty session_id"}`, nil
	}

	output, alive, code := stare(args.SessionID, args.MaxWait)

	if !alive {
		killSession(args.SessionID)
		return fmt.Sprintf(`{"status":"exited","exit_code":%d,"output":%q}`, code, output), nil
	}

	if args.NextCheckAt != "" {
		checkAt, err := parseNextCheckAt(args.NextCheckAt)
		if err == nil {
			setEnv(args.SessionID, "NIK_NEXT_CHECK_AT", checkAt.UTC().Format(time.RFC3339))
		}
	}

	nca, _ := getEnv(args.SessionID, "NIK_NEXT_CHECK_AT")
	return fmt.Sprintf(`{"status":"running","next_check_at":%q,"output":%q}`, nca, output), nil
}

func handleSend(args shellArgs) (string, error) {
	if args.SessionID == "" {
		return `{"error":"empty session_id"}`, nil
	}
	if args.Input == "" {
		return `{"error":"empty input"}`, nil
	}

	err := sendKeys(args.SessionID, args.Input, "Enter")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	output, alive, code := stare(args.SessionID, args.MaxWait)

	if !alive {
		killSession(args.SessionID)
		return fmt.Sprintf(`{"status":"exited","exit_code":%d,"output":%q}`, code, output), nil
	}

	if args.NextCheckAt != "" {
		checkAt, err := parseNextCheckAt(args.NextCheckAt)
		if err == nil {
			setEnv(args.SessionID, "NIK_NEXT_CHECK_AT", checkAt.UTC().Format(time.RFC3339))
		}
	}

	nca, _ := getEnv(args.SessionID, "NIK_NEXT_CHECK_AT")
	return fmt.Sprintf(`{"status":"running","next_check_at":%q,"output":%q}`, nca, output), nil
}

func handleKill(args shellArgs) (string, error) {
	if args.SessionID == "" {
		return `{"error":"empty session_id"}`, nil
	}

	err := killSession(args.SessionID)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	return `{"ok":true}`, nil
}

func handleList() (string, error) {
	sessions, err := listSessions()
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	type sessionEntry struct {
		SessionID      string `json:"session_id"`
		Command        string `json:"command"`
		Description    string `json:"description"`
		ConversationID string `json:"conversation_id"`
		MessageID      string `json:"message_id"`
		Status         string `json:"status"`
		StartedAt      string `json:"started_at"`
		Duration       string `json:"duration"`
		NextCheckAt    string `json:"next_check_at"`
	}

	var entries []sessionEntry
	for _, s := range sessions {
		env, _ := getAllEnv(s.ID)

		status := "running"
		if !s.Alive {
			status = "exited"
		}

		duration := ""
		if startedStr, ok := env["NIK_STARTED_AT"]; ok {
			started, err := time.Parse(time.RFC3339, startedStr)
			if err == nil {
				duration = time.Since(started).Truncate(time.Second).String()
			}
		}

		entries = append(entries, sessionEntry{
			SessionID:      s.ID,
			Command:        env["NIK_COMMAND"],
			Description:    env["NIK_DESCRIPTION"],
			ConversationID: env["NIK_CONVERSATION_ID"],
			MessageID:      env["NIK_MESSAGE_ID"],
			Status:         status,
			StartedAt:      env["NIK_STARTED_AT"],
			Duration:       duration,
			NextCheckAt:    env["NIK_NEXT_CHECK_AT"],
		})
	}

	result, err := json.Marshal(map[string]any{"sessions": entries})
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}

	return string(result), nil
}

// stare polls pane_dead every 500ms, returns early if command exits.
func stare(id string, maxWait int) (output string, alive bool, code int) {
	deadline := time.Now().Add(time.Duration(maxWait) * time.Second)

	for {
		ok, err := isAlive(id)
		if err == nil && !ok {
			out, _ := captureOutput(id)
			c, _ := exitCode(id)
			if c != -1 {
				return out, false, c
			}
		}

		if time.Now().After(deadline) {
			out, _ := captureOutput(id)
			if err == nil && !ok {
				c, _ := exitCode(id)
				return out, false, c
			}
			return out, true, 0
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func parseNextCheckAt(s string) (time.Time, error) {
	if strings.HasPrefix(s, "+") {
		d, err := parseRelativeDuration(s[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("parse relative duration %q: %w", s, err)
		}
		return time.Now().Add(d), nil
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse next_check_at %q: %w", s, err)
	}

	return t, nil
}

func parseRelativeDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		var days int
		_, err := fmt.Sscanf(s, "%dd", &days)
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	return time.ParseDuration(s)
}
