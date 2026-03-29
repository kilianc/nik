package stats

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestMetaFromCtx(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want map[string]string
	}{
		{"empty", context.Background(), nil},
		{"with value", context.WithValue(context.Background(), "meta", map[string]string{"activation_id": "abc"}), map[string]string{"activation_id": "abc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := metaFromCtx(tt.ctx)
			if tt.want == nil {
				if len(m) != 0 {
					t.Fatalf("expected empty map, got %v", m)
				}
				return
			}
			for k, v := range tt.want {
				if m[k] != v {
					t.Fatalf("expected %q=%q, got %q", k, v, m[k])
				}
			}
		})
	}
}

func TestRecorderNoMeta(t *testing.T) {
	r := NewRecorder(nil)
	ctx := context.Background()

	t.Run("start", func(t *testing.T) {
		r.Start(ctx, "gpt-4o")
	})
	t.Run("round", func(t *testing.T) {
		got := r.Round(ctx, 0, 0, "[]", nil, llm.Usage{})
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})
	t.Run("sync", func(t *testing.T) {
		r.Sync(ctx, llm.ActivationStats{})
	})
}

func TestRecorderStart(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	convID := seedStatsConversation(t, ctx, conn)
	actID := id.V7()

	meta := map[string]string{
		"activation_id":   actID,
		"conversation_id": convID,
	}
	ctx = context.WithValue(ctx, "meta", meta)

	r := NewRecorder(conn)
	r.Start(ctx, "gpt-4o")

	var model string
	err = conn.QueryRowContext(ctx, `SELECT model FROM activation WHERE id = ?1`, actID).Scan(&model)
	if err != nil {
		t.Fatalf("query activation: %v", err)
	}
	if model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", model)
	}
}

func TestRecorderRound(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	convID := seedStatsConversation(t, ctx, conn)
	actID := id.V7()

	meta := map[string]string{
		"activation_id":   actID,
		"conversation_id": convID,
	}
	ctx = context.WithValue(ctx, "meta", meta)

	r := NewRecorder(conn)
	r.Start(ctx, "gpt-4o")

	roundID := r.Round(ctx, 0, 0, `[{"role":"user","content":"hello"}]`, []string{"considered"}, llm.Usage{
		InputTokens:  300,
		OutputTokens: 75,
		CachedTokens: 50,
	})
	if roundID == "" {
		t.Fatal("expected non-empty round ID")
	}

	var inputTokens int64
	err = conn.QueryRowContext(ctx, `SELECT input_tokens FROM activation_round WHERE id = ?1`, roundID).Scan(&inputTokens)
	if err != nil {
		t.Fatalf("query round: %v", err)
	}
	if inputTokens != 300 {
		t.Errorf("expected input_tokens 300, got %d", inputTokens)
	}
}

func TestRecorderToolCall(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	convID := seedStatsConversation(t, ctx, conn)
	actID := id.V7()

	meta := map[string]string{
		"activation_id":   actID,
		"conversation_id": convID,
	}
	ctx = context.WithValue(ctx, "meta", meta)

	r := NewRecorder(conn)
	r.Start(ctx, "gpt-4o")
	roundID := r.Round(ctx, 0, 0, "[]", nil, llm.Usage{})

	call := llm.ToolCall{CallID: "c1", Name: "db_query", Arguments: `{"query":"SELECT 1"}`}
	result := llm.ExecResult{Output: `{"rows":[]}`, Elapsed: 42 * time.Millisecond}
	r.ToolCall(ctx, roundID, call, result)

	var name string
	err = conn.QueryRowContext(ctx, `SELECT name FROM tool_call WHERE activation_id = ?1`, actID).Scan(&name)
	if err != nil {
		t.Fatalf("query tool_call: %v", err)
	}
	if name != "db_query" {
		t.Errorf("expected tool name 'db_query', got %q", name)
	}
}

func TestRecorderSync(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	convID := seedStatsConversation(t, ctx, conn)
	actID := id.V7()

	meta := map[string]string{
		"activation_id":   actID,
		"conversation_id": convID,
	}
	ctx = context.WithValue(ctx, "meta", meta)

	r := NewRecorder(conn)
	r.Start(ctx, "gpt-5.4")
	r.Round(ctx, 0, 0, "[]", nil, llm.Usage{InputTokens: 200, OutputTokens: 50})

	r.Sync(ctx, llm.ActivationStats{
		Model:         "gpt-5.4",
		Usage:         llm.Usage{InputTokens: 200, OutputTokens: 50, TotalTokens: 250},
		Rounds:        llm.RoundStats{RoundCount: 1, MaxInputTokensPerRound: 200, MaxTotalTokensPerRound: 250},
		ToolCallCount: 2,
		DurationMS:    800,
	})

	var gotInput, gotDuration int64
	var gotRounds, gotTools int
	err = conn.QueryRowContext(ctx,
		`SELECT input_tokens, duration_ms, round_count, tool_call_count FROM activation WHERE id = ?1`, actID,
	).Scan(&gotInput, &gotDuration, &gotRounds, &gotTools)
	if err != nil {
		t.Fatalf("query activation: %v", err)
	}

	if gotInput != 200 {
		t.Errorf("expected input_tokens 200, got %d", gotInput)
	}
	if gotDuration != 800 {
		t.Errorf("expected duration_ms 800, got %d", gotDuration)
	}
	if gotRounds != 1 {
		t.Errorf("expected round_count 1, got %d", gotRounds)
	}
	if gotTools != 2 {
		t.Errorf("expected tool_call_count 2, got %d", gotTools)
	}

	var gotInstructions string
	err = conn.QueryRowContext(ctx,
		`SELECT instructions FROM activation WHERE id = ?1`, actID,
	).Scan(&gotInstructions)
	if err != nil {
		t.Fatalf("query instructions: %v", err)
	}
	if gotInstructions != "" {
		t.Errorf("expected empty instructions (not set by sync), got %q", gotInstructions)
	}
}

func TestRecorderFinish(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	convID := seedStatsConversation(t, ctx, conn)
	actID := id.V7()

	meta := map[string]string{
		"activation_id":   actID,
		"conversation_id": convID,
	}
	ctx = context.WithValue(ctx, "meta", meta)

	r := NewRecorder(conn)
	r.Start(ctx, "gpt-5.4")

	r.Finish(ctx, llm.ActivationStats{
		Model:           "gpt-5.4",
		ReasoningEffort: "high",
		Usage:           llm.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		Rounds:          llm.RoundStats{RoundCount: 2, MaxInputTokensPerRound: 80, MaxTotalTokensPerRound: 120},
		ToolCallCount:   3,
		DurationMS:      1500,
		Error:           "test error",
		Instructions:    "test instructions",
		Tools:           []string{"db_query", "shell_exec"},
		ToolSchemas: []llm.ToolDef{
			{Name: "db_query", Description: "run a query", Parameters: map[string]any{"type": "object"}},
		},
	})

	var roundCount int
	var gotErr, gotSchemas string
	err = conn.QueryRowContext(ctx,
		`SELECT round_count, error, tool_schemas FROM activation WHERE id = ?1`, actID,
	).Scan(&roundCount, &gotErr, &gotSchemas)
	if err != nil {
		t.Fatalf("query activation: %v", err)
	}
	if roundCount != 2 {
		t.Errorf("expected round_count 2, got %d", roundCount)
	}
	if gotErr != "test error" {
		t.Errorf("expected error 'test error', got %q", gotErr)
	}
	if gotSchemas == "[]" {
		t.Error("expected non-empty tool_schemas")
	}
}

func seedStatsConversation(t *testing.T, ctx context.Context, conn db.DBTX) string {
	t.Helper()

	now := time.Now()
	err := db.ConversationUpsert(ctx, conn, db.ConversationUpsertParams{
		Platform:               "whatsapp",
		ExternalConversationID: "stats-test@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := db.ConversationGet(ctx, conn, db.ConversationGetParams{
		Platform:               "whatsapp",
		ExternalConversationID: "stats-test@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	return conv.ID
}
