package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestExperimentInsertAndGet(t *testing.T) {
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
		DesiredOutcome:    "model should call done",
		Analysis:          "trace analysis here",
	})
	if err != nil {
		t.Fatalf("insert experiment: %v", err)
	}

	got, err := ExperimentGet(ctx, conn, expID)
	if err != nil {
		t.Fatalf("get experiment: %v", err)
	}

	if got.ID != expID {
		t.Fatalf("expected id %q, got %q", expID, got.ID)
	}
	if got.ActivationRoundID != roundID {
		t.Fatalf("expected activation_round_id %q, got %q", roundID, got.ActivationRoundID)
	}
	if got.Status != "analysis" {
		t.Fatalf("expected status %q, got %q", "analysis", got.Status)
	}
	if got.DesiredOutcome != "model should call done" {
		t.Fatalf("expected desired_outcome %q, got %q", "model should call done", got.DesiredOutcome)
	}
	if got.Analysis != "trace analysis here" {
		t.Fatalf("expected analysis %q, got %q", "trace analysis here", got.Analysis)
	}

	gotShort, err := ExperimentGet(ctx, conn, id.Shorten(expID))
	if err != nil {
		t.Fatalf("get experiment by short id: %v", err)
	}
	if gotShort.ID != expID {
		t.Fatalf("short id lookup: expected id %q, got %q", expID, gotShort.ID)
	}
}

func TestExperimentUpdate(t *testing.T) {
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

	newStatus := "experimenting"
	newAnalysis := "updated trace"

	err = ExperimentUpdate(ctx, conn, ExperimentUpdateParams{
		ID:       expID,
		Status:   &newStatus,
		Analysis: &newAnalysis,
	})
	if err != nil {
		t.Fatalf("update experiment: %v", err)
	}

	got, err := ExperimentGet(ctx, conn, expID)
	if err != nil {
		t.Fatalf("get experiment after update: %v", err)
	}

	if got.Status != "experimenting" {
		t.Fatalf("expected status %q, got %q", "experimenting", got.Status)
	}
	if got.Analysis != "updated trace" {
		t.Fatalf("expected analysis %q, got %q", "updated trace", got.Analysis)
	}
}

func seedExperimentRound(t *testing.T, conn DBTX) string {
	t.Helper()

	convID := seedActivationConv(t, conn)
	actID := id.V7()

	err := ActivationInsert(context.Background(), conn, ActivationRow{
		ID:             actID,
		ConversationID: convID,
		Model:          "test-model",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed activation: %v", err)
	}

	roundID, err := ActivationRoundInsert(context.Background(), conn, ActivationRoundInsertParams{
		ActivationID: actID,
		Round:        0,
		UserInput:    "test input",
		ModelOutput:  "test output",
	})
	if err != nil {
		t.Fatalf("seed activation round: %v", err)
	}

	return roundID
}
