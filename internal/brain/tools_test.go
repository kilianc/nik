package brain

import (
	"context"
	"strings"
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

func TestToolExecutorBlocksPrivilegedInUnprivilegedContext(t *testing.T) {
	b := New(&config.Config{PrivilegedConversationIDs: []string{"owner-conv"}}, nil)

	called := false
	b.RegisterTool(llm.Tool{
		Def: llm.ToolDef{Name: "secret_tool"},
		Handler: func(context.Context, llm.ToolCall) (string, error) {
			called = true
			return `{"ok":true}`, nil
		},
		Privileged: true,
	})

	executor := b.toolExecutor()
	call := llm.ToolCall{Name: "secret_tool", Arguments: "{}"}

	unprivCtx := context.WithValue(context.Background(), "meta", map[string]string{"conversation_id": "other"})
	result, err := executor(unprivCtx, call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("privileged tool handler was called in unprivileged context")
	}
	if !strings.Contains(result, "requires privileged context") {
		t.Fatalf("expected privilege error, got %s", result)
	}

	privCtx := context.WithValue(context.Background(), "meta", map[string]string{"conversation_id": "owner-conv"})
	result, err = executor(privCtx, call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("privileged tool handler was not called in privileged context")
	}
	if !strings.Contains(result, `"ok"`) {
		t.Fatalf("expected ok result, got %s", result)
	}
}
