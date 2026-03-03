package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestActivationInsertPersistsRow(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	row := ActivationRow{
		ID:              id.V7(),
		Source:          "messaging",
		SourceID:        "conv-123",
		Model:           "gpt-5",
		ReasoningEffort: "medium",
		InputTokens:     1000,
		OutputTokens:    500,
		TotalTokens:     1500,
		CachedTokens:    200,
		ReasoningTokens: 100,
		CostUSD:         0.0042,
		ToolCallCount:   3,
		DurationMS:      1234,
		Error:           false,
		CreatedAt:       now,
	}

	err = ActivationInsert(ctx, conn, row)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	var got struct {
		id     string
		source string
		model  string
		tokens int64
		errFlg int
	}

	err = conn.QueryRowContext(ctx,
		"SELECT id, source, model, total_tokens, error FROM activation WHERE id = ?1",
		row.ID,
	).Scan(&got.id, &got.source, &got.model, &got.tokens, &got.errFlg)
	if err != nil {
		t.Fatalf("query activation: %v", err)
	}

	if got.id != row.ID {
		t.Fatalf("expected id %q, got %q", row.ID, got.id)
	}
	if got.source != "messaging" {
		t.Fatalf("expected source messaging, got %q", got.source)
	}
	if got.tokens != 1500 {
		t.Fatalf("expected 1500 total_tokens, got %d", got.tokens)
	}
	if got.errFlg != 0 {
		t.Fatalf("expected error=0, got %d", got.errFlg)
	}
}

func TestActivationInsertWithError(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	row := ActivationRow{
		ID:        id.V7(),
		Source:    "alarms",
		Model:     "gpt-5",
		Error:     true,
		CreatedAt: time.Now().UTC(),
	}

	err = ActivationInsert(ctx, conn, row)
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	var errFlg int
	err = conn.QueryRowContext(ctx,
		"SELECT error FROM activation WHERE id = ?1", row.ID,
	).Scan(&errFlg)
	if err != nil {
		t.Fatalf("query activation: %v", err)
	}

	if errFlg != 1 {
		t.Fatalf("expected error=1, got %d", errFlg)
	}
}
