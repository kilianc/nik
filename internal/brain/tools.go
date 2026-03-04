package brain

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/kciuffolo/nik/internal/llm"
)

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
	if len(b.cfg.PrivilegedConversationIDs) == 0 {
		return false
	}

	return slices.Contains(b.cfg.PrivilegedConversationIDs, meta["conversation_id"])
}

func (b *Brain) toolExecutor() llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		if q := reactionQueueFromContext(ctx); q != nil {
			if emoji, ok := b.toolEmojis[call.Name]; ok {
				q.enqueue(emoji)
			}
		}

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
