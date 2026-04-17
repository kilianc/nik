package brain

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
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
		Def:     llm.ToolDef{Name: "private_tool"},
		Handler: handler,
	})
	b.Privileged("private_tool")

	nonOwnerCtx := context.WithValue(context.Background(), "meta", map[string]string{"conversation_id": "other"})
	nonOwnerTools := b.toolsForContext(nonOwnerCtx)
	if len(nonOwnerTools) != 1 {
		t.Fatalf("expected public tool only for non-owner context, got %d tools", len(nonOwnerTools))
	}

	ownerCtx := context.WithValue(context.Background(), "meta", map[string]string{"conversation_id": "owner-conv"})
	ownerTools := b.toolsForContext(ownerCtx)
	if len(ownerTools) != 2 {
		t.Fatalf("expected public + private tools for owner context, got %d tools", len(ownerTools))
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
	})
	b.Privileged("secret_tool")

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

func seedConversation(t *testing.T, conn *sql.DB) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	err := db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "local",
		ExternalConversationID: "test-conv",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "local",
		ExternalConversationID: "test-conv",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	return conv.ID
}

func TestInsertToolCallStartAndToolCallMessages(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	err = db.SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convID := seedConversation(t, conn)
	cfg := &config.Config{Home: t.TempDir()}
	b := New(cfg, nil, prompt.NewRenderer(cfg))
	b.conn = conn

	now := time.Now().UTC().Truncate(time.Millisecond)
	calls := []llm.ToolCall{
		{Name: "shell", Arguments: `{"action":"run"}`},
		{Name: "db_query", Arguments: `{"reason":"check"}`},
	}

	startIDs := b.insertToolCallStartMessages(ctx, convID, 1, calls, now)
	if len(startIDs) != 2 {
		t.Fatalf("expected 2 start IDs, got %d", len(startIDs))
	}
	for i, id := range startIDs {
		if id == "" {
			t.Errorf("start ID %d is empty", i)
		}
	}

	results := []llm.ExecResult{
		{Output: `{"exit_code":0}`},
		{Output: `{"rows":[]}`},
	}
	b.insertToolCallMessages(ctx, convID, 1, startIDs, calls, results, now)

	// verify tool_call messages link back via context_stanza_id
	for i, startID := range startIDs {
		var stanzaID sql.NullString
		var body string
		err = conn.QueryRowContext(ctx,
			`SELECT context_stanza_id, body FROM message WHERE kind = 'tool_call' AND context_stanza_id = ?1`,
			startID,
		).Scan(&stanzaID, &body)
		if err != nil {
			t.Fatalf("query tool_call for start %d: %v", i, err)
		}

		var tc ToolCallBody
		json.Unmarshal([]byte(body), &tc)
		if tc.Name != calls[i].Name {
			t.Errorf("tool_call %d name = %q, want %q", i, tc.Name, calls[i].Name)
		}
		if tc.Output != results[i].Output {
			t.Errorf("tool_call %d output = %q, want %q", i, tc.Output, results[i].Output)
		}
	}
}

func TestIsDone(t *testing.T) {
	tests := []struct {
		name  string
		calls []llm.ToolCall
		want  bool
	}{
		{"done tool", []llm.ToolCall{{Name: "done"}}, true},
		{"done among others", []llm.ToolCall{{Name: "message_send"}, {Name: "done"}}, true},
		{"no done tool", []llm.ToolCall{{Name: "message_send"}}, false},
		{"empty", []llm.ToolCall{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDone(tt.calls)
			if got != tt.want {
				t.Fatalf("isDone() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInsertToolCallStartNilConn(t *testing.T) {
	cfg := &config.Config{Home: t.TempDir()}
	b := New(cfg, nil, prompt.NewRenderer(cfg))

	ids := b.insertToolCallStartMessages(context.Background(), "conv", 1, []llm.ToolCall{{Name: "x"}}, time.Now())
	if ids != nil {
		t.Errorf("expected nil IDs with nil conn, got %v", ids)
	}
}
