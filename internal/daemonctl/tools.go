package daemonctl

import (
	"context"
	"log/slog"
	"os"
	"syscall"

	"github.com/kciuffolo/nik/internal/llm"
)

var RestartToolDef = llm.ToolDef{
	Name:        "restart",
	Description: "Request a graceful daemon restart. The current activation completes, then nik restarts with fresh configuration.",
	Parameters: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []string{},
		"additionalProperties": false,
	},
}

func RestartHandler() llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		slog.Info("restart requested, sending SIGTERM", "pkg", "daemonctl")
		syscall.Kill(os.Getpid(), syscall.SIGTERM)
		return `{"ok":true,"message":"restart scheduled after current activation completes"}`, nil
	}
}
