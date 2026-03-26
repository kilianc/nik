package db

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/id"
)

func TestExperimentVariantRunInsertAndList(t *testing.T) {
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
		Patches:      "",
	})
	if err != nil {
		t.Fatalf("insert variant: %v", err)
	}

	run1, err := ExperimentVariantRunInsert(ctx, conn, varID)
	if err != nil {
		t.Fatalf("insert run 1: %v", err)
	}

	if run1.ID == "" {
		t.Fatal("expected non-empty run ID")
	}
	if run1.ExperimentVariantID != varID {
		t.Fatalf("expected variant_id %q, got %q", varID, run1.ExperimentVariantID)
	}
	if run1.ToolCalls != "[]" {
		t.Fatalf("expected default tool_calls, got %q", run1.ToolCalls)
	}

	run1.ToolCalls = `[{"name":"message_noop"}]`
	run1.ReasoningSummaries = `["considered noop"]`
	run1.InputTokens = 4521
	run1.OutputTokens = 89
	run1.CachedTokens = 200
	run1.ReasoningTokens = 50

	err = ExperimentVariantRunSaveResult(ctx, conn, run1)
	if err != nil {
		t.Fatalf("save run 1 result: %v", err)
	}

	run2, err := ExperimentVariantRunInsert(ctx, conn, varID)
	if err != nil {
		t.Fatalf("insert run 2: %v", err)
	}

	run2.ToolCalls = `[{"name":"task_spawn"}]`
	run2.ModelOutput = "On it."
	run2.ReasoningSummaries = `["acknowledged request"]`
	run2.InputTokens = 4521
	run2.OutputTokens = 234

	err = ExperimentVariantRunSaveResult(ctx, conn, run2)
	if err != nil {
		t.Fatalf("save run 2 result: %v", err)
	}

	runs, err := ExperimentVariantRunList(ctx, conn, varID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}

	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	if runs[0].ID != run1.ID {
		t.Fatalf("expected first run id %q, got %q", run1.ID, runs[0].ID)
	}
	if runs[0].IsDesired != nil {
		t.Fatalf("expected is_desired nil (unassessed), got %v", *runs[0].IsDesired)
	}
	if runs[0].InputTokens != 4521 {
		t.Fatalf("expected input_tokens 4521, got %d", runs[0].InputTokens)
	}
	if runs[0].ToolCalls != `[{"name":"message_noop"}]` {
		t.Fatalf("expected tool_calls %q, got %q", `[{"name":"message_noop"}]`, runs[0].ToolCalls)
	}

	if runs[1].IsDesired != nil {
		t.Fatalf("expected second run is_desired nil, got %v", *runs[1].IsDesired)
	}
	if runs[1].ModelOutput != "On it." {
		t.Fatalf("expected model_output %q, got %q", "On it.", runs[1].ModelOutput)
	}

	gotVariantID, err := ExperimentVariantRunUpdate(ctx, conn, run1.ID, true, "both tools called")
	if err != nil {
		t.Fatalf("update desired: %v", err)
	}

	if gotVariantID != varID {
		t.Fatalf("expected variant ID %q, got %q", varID, gotVariantID)
	}

	runs, err = ExperimentVariantRunList(ctx, conn, varID)
	if err != nil {
		t.Fatalf("list runs after update: %v", err)
	}

	if runs[0].IsDesired == nil || !*runs[0].IsDesired {
		t.Fatal("expected run to be desired after update")
	}
	if runs[0].Rationale != "both tools called" {
		t.Fatalf("expected rationale %q, got %q", "both tools called", runs[0].Rationale)
	}

	_, err = ExperimentVariantRunUpdate(ctx, conn, run1.ID, false, "only sent message")
	if err != nil {
		t.Fatalf("update desired back to false: %v", err)
	}

	runs, err = ExperimentVariantRunList(ctx, conn, varID)
	if err != nil {
		t.Fatalf("list runs after second update: %v", err)
	}

	if runs[0].IsDesired == nil || *runs[0].IsDesired {
		t.Fatal("expected run to not be desired after second update")
	}
	if runs[0].Rationale != "only sent message" {
		t.Fatalf("expected rationale %q, got %q", "only sent message", runs[0].Rationale)
	}
}

func TestExperimentVariantRunGet(t *testing.T) {
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
		ID:              varID,
		ExperimentID:    expID,
		Name:            "baseline",
		Patches:         "",
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("insert variant: %v", err)
	}

	bare, err := ExperimentVariantRunInsert(ctx, conn, varID)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	run, err := ExperimentVariantRunGet(ctx, conn, bare.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}

	if run.ID != bare.ID {
		t.Fatalf("expected id %q, got %q", bare.ID, run.ID)
	}
	if run.Model != "test-model" {
		t.Fatalf("expected model %q, got %q", "test-model", run.Model)
	}
	if run.UserInput != "test input" {
		t.Fatalf("expected user_input %q, got %q", "test input", run.UserInput)
	}
	if run.ReasoningEffort != "high" {
		t.Fatalf("expected reasoning_effort %q, got %q", "high", run.ReasoningEffort)
	}
	if run.ToolCalls != "[]" {
		t.Fatalf("expected default tool_calls, got %q", run.ToolCalls)
	}
}
