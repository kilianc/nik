package db

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/id"
)

func TestExperimentRunInsertAndList(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	roundID := seedExperimentRound(t, conn)
	expID := id.V7()

	err = ExperimentInsert(ctx, conn, ExperimentInsertParams{
		ID:                expID,
		ActivationRoundID: roundID,
		Status:            "experimenting",
	})
	if err != nil {
		t.Fatalf("insert experiment: %v", err)
	}

	varID := id.V7()

	err = ExperimentVariantInsert(ctx, conn, ExperimentVariantInsertParams{
		ID:           varID,
		ExperimentID: expID,
		Name:         "baseline",
		Status:       "running",
		Patches:      "[]",
	})
	if err != nil {
		t.Fatalf("insert variant: %v", err)
	}

	runID1, err := ExperimentRunInsert(ctx, conn, ExperimentRunInsertParams{
		ExperimentVariantID: varID,
		ToolCalls:           `[{"name":"message_noop"}]`,
		ModelOutput:         "",
		ReasoningSummaries:  `["considered noop"]`,
		IsDesired:           true,
		InputTokens:         4521,
		OutputTokens:        89,
		CachedTokens:        200,
		ReasoningTokens:     50,
	})
	if err != nil {
		t.Fatalf("insert run 1: %v", err)
	}

	if runID1 == "" {
		t.Fatal("expected non-empty run ID")
	}

	runID2, err := ExperimentRunInsert(ctx, conn, ExperimentRunInsertParams{
		ExperimentVariantID: varID,
		ToolCalls:           `[{"name":"task_spawn"}]`,
		ModelOutput:         "On it.",
		ReasoningSummaries:  `["acknowledged request"]`,
		IsDesired:           false,
		InputTokens:         4521,
		OutputTokens:        234,
	})
	if err != nil {
		t.Fatalf("insert run 2: %v", err)
	}

	if runID2 == "" {
		t.Fatal("expected non-empty run ID")
	}

	runs, err := ExperimentRunList(ctx, conn, varID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}

	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	if runs[0].ID != runID1 {
		t.Fatalf("expected first run id %q, got %q", runID1, runs[0].ID)
	}
	if !runs[0].IsDesired {
		t.Fatal("expected first run to be desired")
	}
	if runs[0].InputTokens != 4521 {
		t.Fatalf("expected input_tokens 4521, got %d", runs[0].InputTokens)
	}
	if runs[0].OutputTokens != 89 {
		t.Fatalf("expected output_tokens 89, got %d", runs[0].OutputTokens)
	}
	if runs[0].ToolCalls != `[{"name":"message_noop"}]` {
		t.Fatalf("expected tool_calls %q, got %q", `[{"name":"message_noop"}]`, runs[0].ToolCalls)
	}

	if runs[1].IsDesired {
		t.Fatal("expected second run to not be desired")
	}
	if runs[1].ModelOutput != "On it." {
		t.Fatalf("expected model_output %q, got %q", "On it.", runs[1].ModelOutput)
	}
}
