package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

type ToolCallBody struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Round  int    `json:"round"`
}

const doneToolName = "done"

var doneToolDef = llm.ToolDef{
	Name:        doneToolName,
	Description: "Signal that you are done with this activation. Do all your work first -- messages, reactions, tasks, lookups -- then call done to declare completion.",
	Parameters: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []string{},
		"additionalProperties": false,
	},
}

func doneHandler() llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			Reason string `json:"reason"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		slog.Info("done", "pkg", "brain", "reason", args.Reason)
		return `{"ok":true}`, nil
	}
}

func (b *Brain) RegisterTool(t llm.Tool) {
	if t.Def.Name == "" {
		panic("register tool: empty name")
	}
	if t.Handler == nil {
		panic(fmt.Sprintf("register tool %s: nil handler", t.Def.Name))
	}
	if _, exists := b.toolExec[t.Def.Name]; exists {
		panic(fmt.Sprintf("register tool %s: already registered", t.Def.Name))
	}

	b.toolDefs = append(b.toolDefs, t.Def)
	b.toolExec[t.Def.Name] = t.Handler
	if t.Privileged {
		b.privileged[t.Def.Name] = true
		slog.Info("registered privileged tool", "pkg", "brain", "tool", t.Def.Name)
	}
}

func (b *Brain) RegisterTools(ts ...llm.Tool) {
	for _, t := range ts {
		b.RegisterTool(t)
	}
}

func (b *Brain) ToolNames() []string {
	names := make([]string, 0, len(b.toolDefs))
	for _, d := range b.toolDefs {
		names = append(names, d.Name)
	}
	return names
}

func (b *Brain) toolsForContext(ctx context.Context) []llm.ToolDef {
	meta, _ := ctx.Value("meta").(map[string]string)
	isPrivileged := b.isPrivilegedContext(meta)

	var defs []llm.ToolDef
	for _, def := range b.toolDefs {
		if b.privileged[def.Name] && !isPrivileged {
			continue
		}
		defs = append(defs, def)
	}

	slog.Debug("tools for context",
		"pkg", "brain",
		"conversation_id", meta["conversation_id"],
		"privileged_conversation_ids", b.cfg.PrivilegedConversationIDs,
		"is_privileged", isPrivileged,
		"total_registered", len(b.toolDefs),
		"privileged_registered", len(b.privileged),
		"tools_sent", len(defs),
	)

	return defs
}

func (b *Brain) isPrivilegedContext(meta map[string]string) bool {
	return b.cfg.IsPrivileged(meta["conversation_id"])
}

func (b *Brain) insertToolCallMessages(ctx context.Context, convID string, round int, calls []llm.ToolCall, results []llm.ExecResult) {
	if b.conn == nil {
		return
	}

	now := time.Now().UTC()
	for i, call := range calls {
		body := ToolCallBody{
			Name:   call.Name,
			Input:  call.Arguments,
			Output: results[i].Output,
			Round:  round,
		}

		err := db.SystemMessageInsert(ctx, b.conn, db.SystemMessageParams{
			ConversationID: convID,
			Kind:           "tool_call",
			Body:           body,
			SentAt:         now,
		})
		if err != nil {
			slog.Warn("insert tool call message", "pkg", "brain", "tool", call.Name, "error", err)
		}
	}
}

func (b *Brain) toolExecutor() llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		handler, ok := b.toolExec[call.Name]
		if !ok {
			return llm.ToolErrorf("unknown tool %q", call.Name), nil
		}

		if b.privileged[call.Name] {
			meta, _ := ctx.Value("meta").(map[string]string)
			if !b.isPrivilegedContext(meta) {
				slog.Warn("blocked privileged tool in unprivileged context",
					"pkg", "brain", "tool", call.Name,
					"conversation_id", meta["conversation_id"],
				)
				return llm.ToolErrorf("tool %q requires privileged context", call.Name), nil
			}
		}

		return handler(ctx, call)
	}
}
