package brain

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestRegisterToolPanicsOnEmptyName(t *testing.T) {
	b := New(&config.Config{}, nil)

	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for empty tool name")
		}
	}()

	b.RegisterTool(llm.Tool{
		Def: llm.ToolDef{},
		Handler: func(context.Context, llm.ToolCall) (string, error) {
			return "", nil
		},
	})
}

func TestToolsForContextFiltersPrivilegedTools(t *testing.T) {
	b := New(&config.Config{PrivilegedConversationIDs: []string{"owner-conv"}}, nil)
	handler := func(context.Context, llm.ToolCall) (string, error) { return `{"ok":true}`, nil }

	b.RegisterTool(llm.Tool{
		Def:     llm.ToolDef{Name: "public_tool"},
		Handler: handler,
	})
	b.RegisterTool(llm.Tool{
		Def:        llm.ToolDef{Name: "private_tool"},
		Handler:    handler,
		Privileged: true,
	})

	nonOwnerCtx := context.WithValue(context.Background(), "meta", map[string]string{"conversation_id": "other"})
	nonOwnerTools := b.toolsForContext(nonOwnerCtx)
	if len(nonOwnerTools) != 1 || nonOwnerTools[0].Name != "public_tool" {
		t.Fatalf("expected only public tool for non-owner context, got %+v", nonOwnerTools)
	}

	ownerCtx := context.WithValue(context.Background(), "meta", map[string]string{"conversation_id": "owner-conv"})
	ownerTools := b.toolsForContext(ownerCtx)
	if len(ownerTools) != 2 {
		t.Fatalf("expected both tools for owner context, got %+v", ownerTools)
	}
}
