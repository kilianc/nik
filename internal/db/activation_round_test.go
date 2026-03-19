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

	roundID, err := ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
		ActivationID:       actID,
		Round:              0,
		UserInput:          "hello world",
		ModelOutput:        "thinking...",
		ReasoningSummaries: []string{"considered greeting"},
	})
	if err != nil {
		t.Fatalf("insert round 0: %v", err)
	}

	if roundID == "" {
		t.Fatal("expected non-empty round ID")
	}

	var got struct {
		round              int
		userInput          string
		modelOutput        string
		reasoningSummaries string
	}

	err = conn.QueryRowContext(ctx,
		"SELECT round, user_input, model_output, reasoning_summaries FROM activation_round WHERE id = ?1",
		roundID,
	).Scan(&got.round, &got.userInput, &got.modelOutput, &got.reasoningSummaries)
	if err != nil {
		t.Fatalf("query activation round: %v", err)
	}

	if got.round != 0 {
		t.Fatalf("expected round 0, got %d", got.round)
	}
	if got.userInput != "hello world" {
		t.Fatalf("expected user_input %q, got %q", "hello world", got.userInput)
	}
	if got.modelOutput != "thinking..." {
		t.Fatalf("expected model_output %q, got %q", "thinking...", got.modelOutput)
	}
	if got.reasoningSummaries != `["considered greeting"]` {
		t.Fatalf("expected reasoning_summaries %q, got %q", `["considered greeting"]`, got.reasoningSummaries)
	}

	_, err = ActivationRoundInsert(ctx, conn, ActivationRoundInsertParams{
		ActivationID: actID,
		Round:        1,
		UserInput:    "round 1 input",
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
