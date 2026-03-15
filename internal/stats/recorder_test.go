package stats

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

func TestMetaFromCtxReturnsEmpty(t *testing.T) {
	m := metaFromCtx(context.Background())
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
}

func TestMetaFromCtxReturnsValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), "meta", map[string]string{"activation_id": "abc"})
	m := metaFromCtx(ctx)
	if m["activation_id"] != "abc" {
		t.Fatalf("expected 'abc', got %q", m["activation_id"])
	}
}

func TestRecorderOnStartNoMetaNoops(t *testing.T) {
	r := NewRecorder(nil)
	r.OnStart(context.Background(), "gpt-4o")
}

func TestRecorderOnStartWritesActivation(t *testing.T) {
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
