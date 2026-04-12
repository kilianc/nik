package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

func TestToolCallStartRecover(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	err = SystemContactEnsure(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn, "local", "conv-1", "dm")

	startID, err := SystemMessageInsert(ctx, conn, SystemMessageParams{
		ConversationID: convID,
		Kind:           "tool_call_start",
		Body:           map[string]any{"name": "shell", "input": `{"action":"run"}`, "round": 1},
		SentAt:         now,
	})
	if err != nil {
		t.Fatalf("insert tool_call_start: %v", err)
	}

	pairedStartID, err := SystemMessageInsert(ctx, conn, SystemMessageParams{
		ConversationID: convID,
		Kind:           "tool_call_start",
		Body:           map[string]any{"name": "db_query", "input": `{"reason":"check"}`, "round": 1},
		SentAt:         now,
	})
	if err != nil {
		t.Fatalf("insert paired tool_call_start: %v", err)
	}

	_, err = SystemMessageInsert(ctx, conn, SystemMessageParams{
		ConversationID:  convID,
		Kind:            "tool_call",
		Body:            map[string]any{"name": "db_query", "input": `{"reason":"check"}`, "output": "ok", "round": 1},
		SentAt:          now,
		ContextStanzaID: pairedStartID,
	})
	if err != nil {
		t.Fatalf("insert paired tool_call: %v", err)
	}

	err = ToolCallStartRecover(ctx, conn)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	// verify a tool_call was inserted for the orphan
	var body string
	var stanzaID sql.NullString
	err = conn.QueryRowContext(ctx,
		`SELECT body, context_stanza_id FROM message WHERE kind = 'tool_call' AND context_stanza_id = ?1`,
		startID,
	).Scan(&body, &stanzaID)
	if err != nil {
		t.Fatalf("query recovery tool_call: %v", err)
	}

	var tc struct {
		Name   string `json:"name"`
		Output string `json:"output"`
	}
	json.Unmarshal([]byte(body), &tc)

	if tc.Name != "shell" {
		t.Errorf("recovered tool_call name = %q, want %q", tc.Name, "shell")
	}
	if tc.Output != `{"error":"interrupted"}` {
		t.Errorf("recovered tool_call output = %q, want interrupted error", tc.Output)
	}

	// verify the already-paired start was not double-recovered
	var count int
	err = conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM message WHERE kind = 'tool_call' AND context_stanza_id = ?1`,
		pairedStartID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count paired: %v", err)
	}
	if count != 1 {
		t.Errorf("paired start should have exactly 1 tool_call, got %d", count)
	}
}

func TestToolCallStartRecoverEmpty(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = ToolCallStartRecover(context.Background(), conn)
	if err != nil {
		t.Fatalf("recover on empty db: %v", err)
	}
}
