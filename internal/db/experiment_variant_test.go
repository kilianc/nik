package db

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/id"
)

func TestExperimentVariantInsertAndGet(t *testing.T) {
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
		Status:            "analysis",
	})
	if err != nil {
		t.Fatalf("insert experiment: %v", err)
	}

	varID := id.V7()
	patches := "--- a/instructions\n+++ b/instructions\n@@ -1,1 +1,1 @@\n-old text\n+new text\n"

	err = ExperimentVariantInsert(ctx, conn, ExperimentVariantInsertParams{
		ID:              varID,
		ExperimentID:    expID,
		Name:            "shorter-ack",
		Hypothesis:      "adding a noop rule will reduce duplicates",
		Patches:         patches,
		ReasoningEffort: "medium",
		Verbosity:       "low",
	})
	if err != nil {
		t.Fatalf("insert variant: %v", err)
	}

	got, err := ExperimentVariantGet(ctx, conn, varID)
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}

	if got.ID != varID {
		t.Fatalf("expected id %q, got %q", varID, got.ID)
	}
	if got.ExperimentID != expID {
		t.Fatalf("expected experiment_id %q, got %q", expID, got.ExperimentID)
	}
	if got.Name != "shorter-ack" {
		t.Fatalf("expected name %q, got %q", "shorter-ack", got.Name)
	}
	if got.Hypothesis != "adding a noop rule will reduce duplicates" {
		t.Fatalf("expected hypothesis %q, got %q", "adding a noop rule will reduce duplicates", got.Hypothesis)
	}
	if got.Patches != patches {
		t.Fatalf("expected patches %q, got %q", patches, got.Patches)
	}
	if got.ReasoningEffort != "medium" {
		t.Fatalf("expected reasoning_effort %q, got %q", "medium", got.ReasoningEffort)
	}
	if got.Verbosity != "low" {
		t.Fatalf("expected verbosity %q, got %q", "low", got.Verbosity)
	}
}

func TestExperimentVariantList(t *testing.T) {
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

	err = ExperimentVariantInsert(ctx, conn, ExperimentVariantInsertParams{
		ID:           id.V7(),
		ExperimentID: expID,
		Name:         "baseline",
		Patches:      "",
	})
	if err != nil {
		t.Fatalf("insert baseline variant: %v", err)
	}

	err = ExperimentVariantInsert(ctx, conn, ExperimentVariantInsertParams{
		ID:           id.V7(),
		ExperimentID: expID,
		Name:         "variant-1",
		Hypothesis:   "test hypothesis",
		Patches:      "--- a/instructions\n+++ b/instructions\n@@ -1,1 +1,1 @@\n-a\n+b\n",
	})
	if err != nil {
		t.Fatalf("insert variant-1: %v", err)
	}

	variants, err := ExperimentVariantList(ctx, conn, expID)
	if err != nil {
		t.Fatalf("list variants: %v", err)
	}

	if len(variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(variants))
	}

	if variants[0].Name != "baseline" {
		t.Fatalf("expected first variant name %q, got %q", "baseline", variants[0].Name)
	}
	if variants[1].Name != "variant-1" {
		t.Fatalf("expected second variant name %q, got %q", "variant-1", variants[1].Name)
	}
}

func TestExperimentVariantUpdate(t *testing.T) {
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

	newRunCount := 10
	newDesiredCount := 7

	err = ExperimentVariantUpdate(ctx, conn, ExperimentVariantUpdateParams{
		ID:           varID,
		RunCount:     &newRunCount,
		DesiredCount: &newDesiredCount,
	})
	if err != nil {
		t.Fatalf("update variant: %v", err)
	}

	got, err := ExperimentVariantGet(ctx, conn, varID)
	if err != nil {
		t.Fatalf("get variant after update: %v", err)
	}

	if got.RunCount != 10 {
		t.Fatalf("expected run_count 10, got %d", got.RunCount)
	}
	if got.DesiredCount != 7 {
		t.Fatalf("expected desired_count 7, got %d", got.DesiredCount)
	}
}
