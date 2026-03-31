package brain

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/prompt"
)

func TestRegisterToolPanicsOnEmptyName(t *testing.T) {
	b := New(&config.Config{}, nil, prompt.NewRenderer(&config.Config{Home: t.TempDir()}))

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
	b := New(&config.Config{PrivilegedConversationIDs: config.ConversationList{{Label: "owner", ID: "owner-conv"}}}, nil, prompt.NewRenderer(&config.Config{Home: t.TempDir()}))
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
	if len(nonOwnerTools) != 2 {
		t.Fatalf("expected done + public tool for non-owner context, got %d tools", len(nonOwnerTools))
	}

	ownerCtx := context.WithValue(context.Background(), "meta", map[string]string{"conversation_id": "owner-conv"})
	ownerTools := b.toolsForContext(ownerCtx)
	if len(ownerTools) != 3 {
		t.Fatalf("expected done + public + private tools for owner context, got %d tools", len(ownerTools))
	}
}

func TestToolExecutorBlocksPrivilegedInUnprivilegedContext(t *testing.T) {
	b := New(&config.Config{PrivilegedConversationIDs: config.ConversationList{{Label: "owner", ID: "owner-conv"}}}, nil, prompt.NewRenderer(&config.Config{Home: t.TempDir()}))

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
