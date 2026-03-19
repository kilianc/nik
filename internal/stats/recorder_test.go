package stats

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
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

func TestRecorderOnStart(t *testing.T) {
	t.Run("no meta noops", func(t *testing.T) {
		r := NewRecorder(nil)
		r.OnStart(context.Background(), "gpt-4o")
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
		r.OnStart(ctx, "gpt-4o")

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

func TestRecorderOnRound(t *testing.T) {
	t.Run("no meta noops", func(t *testing.T) {
		r := NewRecorder(nil)
		got := r.OnRound(context.Background(), 0, "input", "output", nil)
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
		r.OnStart(ctx, "gpt-4o")

		roundID := r.OnRound(ctx, 0, "hello", "thinking", []string{"considered"})
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
