package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestActivationRoundInsert(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedActivationConv(t, conn)
	actID := id.V7()

	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:             actID,
		ConversationID: convID,
		Model:          "test-model",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	msgs := `[{"role":"user","content":"hello world"}]`
	roundID, err := ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
		ActivationID:       actID,
		Round:              0,
		Messages:           msgs,
		ReasoningSummaries: []string{"considered greeting"},
		InputTokens:        500,
		OutputTokens:       120,
		CachedTokens:       80,
		ReasoningTokens:    40,
	})
	if err != nil {
		t.Fatalf("insert round 0: %v", err)
	}

	if roundID == "" {
		t.Fatal("expected non-empty round ID")
	}

	var got struct {
		round              int
		messages           string
		reasoningSummaries string
		inputTokens        int64
		outputTokens       int64
		cachedTokens       int64
		reasoningTokens    int64
	}

	err = conn.QueryRowContext(ctx,
		`SELECT round, messages, reasoning_summaries,
		  input_tokens, output_tokens, cached_tokens, reasoning_tokens
		FROM activation_round WHERE id = ?1`,
		roundID,
	).Scan(
		&got.round,
		&got.messages,
		&got.reasoningSummaries,
		&got.inputTokens,
		&got.outputTokens,
		&got.cachedTokens,
		&got.reasoningTokens,
	)
	if err != nil {
		t.Fatalf("query activation round: %v", err)
	}

	if got.round != 0 {
		t.Fatalf("expected round 0, got %d", got.round)
	}
	if got.messages != msgs {
		t.Fatalf("expected messages %q, got %q", msgs, got.messages)
	}
	if got.reasoningSummaries != `["considered greeting"]` {
		t.Fatalf("expected reasoning_summaries %q, got %q", `["considered greeting"]`, got.reasoningSummaries)
	}
	if got.inputTokens != 500 {
		t.Fatalf("expected input_tokens 500, got %d", got.inputTokens)
	}
	if got.outputTokens != 120 {
		t.Fatalf("expected output_tokens 120, got %d", got.outputTokens)
	}
	if got.cachedTokens != 80 {
		t.Fatalf("expected cached_tokens 80, got %d", got.cachedTokens)
	}
	if got.reasoningTokens != 40 {
		t.Fatalf("expected reasoning_tokens 40, got %d", got.reasoningTokens)
	}

	_, err = ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
		ActivationID: actID,
		Round:        1,
	})
	if err != nil {
		t.Fatalf("insert round 1: %v", err)
	}

	var count int
	err = conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM activation_round WHERE activation_id = ?1", actID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count rounds: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 rounds, got %d", count)
	}
}

func TestActivationRoundGet(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedActivationConv(t, conn)
	actID := id.V7()

	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:             actID,
		ConversationID: convID,
		Model:          "test-model",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	roundID, err := ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
		ActivationID: actID,
		Round:        0,
	})
	if err != nil {
		t.Fatalf("insert round: %v", err)
	}

	got, err := ActivationRoundGet(ctx, conn, roundID)
	if err != nil {
		t.Fatalf("get round: %v", err)
	}

	if got.ActivationID != actID {
		t.Fatalf("expected activation_id %q, got %q", actID, got.ActivationID)
	}
	if got.Round != 0 {
		t.Fatalf("expected round 0, got %d", got.Round)
	}
}

func TestActivationRoundList(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedActivationConv(t, conn)
	actID := id.V7()

	err = ActivationInsert(ctx, conn, ActivationRow{
		ID:             actID,
		ConversationID: convID,
		Model:          "test-model",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert activation: %v", err)
	}

	for i := range 3 {
		_, err = ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
			ActivationID: actID,
			Round:        i,
		})
		if err != nil {
			t.Fatalf("insert round %d: %v", i, err)
		}
	}

	all, err := ActivationRoundList(ctx, conn, actID, nil)
	if err != nil {
		t.Fatalf("list all rounds: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 rounds, got %d", len(all))
	}

	beforeTwo := 2
	prior, err := ActivationRoundList(ctx, conn, actID, &beforeTwo)
	if err != nil {
		t.Fatalf("list prior rounds: %v", err)
	}
	if len(prior) != 2 {
		t.Fatalf("expected 2 rounds before round 2, got %d", len(prior))
	}
}
