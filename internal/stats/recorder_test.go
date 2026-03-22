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

func TestRecorderStart(t *testing.T) {
	t.Run("no meta noops", func(t *testing.T) {
		r := NewRecorder(nil)
		r.Start(context.Background(), "gpt-4o")
	})

	t.Run("writes activation", func(t *testing.T) {
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
	})
}

func TestRecorderRound(t *testing.T) {
	t.Run("no meta noops", func(t *testing.T) {
		r := NewRecorder(nil)
		got := r.Round(context.Background(), 0, 0, "input", "output", nil)
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("writes round", func(t *testing.T) {
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

		roundID := r.Round(ctx, 0, 0, "hello", "thinking", []string{"considered"})
		if roundID == "" {
			t.Fatal("expected non-empty round ID")
		}

		var userInput string
		err = conn.QueryRowContext(ctx, `SELECT user_input FROM activation_round WHERE id = ?1`, roundID).Scan(&userInput)
		if err != nil {
			t.Fatalf("query round: %v", err)
		}
		if userInput != "hello" {
			t.Errorf("expected user_input 'hello', got %q", userInput)
		}
	})
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
	roundID := r.Round(ctx, 0, 0, "input", "output", nil)

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
		Instructions:    "test instructions",
		Tools:           []string{"db_query", "shell_exec"},
	})

	var roundCount int
	err = conn.QueryRowContext(ctx, `SELECT round_count FROM activation WHERE id = ?1`, actID).Scan(&roundCount)
	if err != nil {
		t.Fatalf("query activation: %v", err)
	}
	if roundCount != 2 {
		t.Errorf("expected round_count 2, got %d", roundCount)
	}
}

func seedStatsConversation(t *testing.T, ctx context.Context, conn db.DBTX) string {
	t.Helper()

	now := time.Now()
	err := db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "stats-test@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "stats-test@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	return conv.ID
}
