package workbench

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

func TestCreateExperiment(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "model should noop")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	if expID == "" {
		t.Fatal("expected non-empty experiment ID")
	}

	exp, err := db.ExperimentGet(ctx, conn, expID)
	if err != nil {
		t.Fatalf("get experiment: %v", err)
	}

	if exp.Status != "analysis" {
		t.Fatalf("expected status %q, got %q", "analysis", exp.Status)
	}

	variants, err := db.ExperimentVariantList(ctx, conn, expID)
	if err != nil {
		t.Fatalf("list variants: %v", err)
	}

	if len(variants) != 1 {
		t.Fatalf("expected 1 baseline variant, got %d", len(variants))
	}

	if variants[0].Name != "baseline" {
		t.Fatalf("expected baseline variant, got %q", variants[0].Name)
	}

	if variants[0].Patches != "[]" {
		t.Fatalf("expected empty patches, got %q", variants[0].Patches)
	}
}

func TestCreateVariant(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired outcome")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	patches := []Patch{{File: "prompts/brain.md", Old: "old", New: "new"}}
	varID, err := CreateVariant(ctx, conn, expID, "shorter-ack", "reduce duplicates", patches, "medium", "low")
	if err != nil {
		t.Fatalf("create variant: %v", err)
	}

	v, err := db.ExperimentVariantGet(ctx, conn, varID)
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}

	if v.Name != "shorter-ack" {
		t.Fatalf("expected name %q, got %q", "shorter-ack", v.Name)
	}
	if v.Status != "proposed" {
		t.Fatalf("expected status %q, got %q", "proposed", v.Status)
	}
}

func TestRecordRun(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired outcome")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	variants, err := db.ExperimentVariantList(ctx, conn, expID)
	if err != nil {
		t.Fatalf("list variants: %v", err)
	}

	baselineID := variants[0].ID
	result := ReplayResult{
		ToolCalls:       []ToolCall{{Name: "message_noop"}},
		ModelOutput:     "",
		InputTokens:     4521,
		OutputTokens:    89,
		CachedTokens:    200,
		ReasoningTokens: 50,
	}

	runID, err := RecordRun(ctx, conn, baselineID, result, true)
	if err != nil {
		t.Fatalf("record run: %v", err)
	}

	if runID == "" {
		t.Fatal("expected non-empty run ID")
	}

	v, err := db.ExperimentVariantGet(ctx, conn, baselineID)
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}

	if v.RunCount != 1 {
		t.Fatalf("expected run_count 1, got %d", v.RunCount)
	}
	if v.DesiredCount != 1 {
		t.Fatalf("expected desired_count 1, got %d", v.DesiredCount)
	}
}

func TestGetStatus(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired outcome")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	status, err := GetStatus(ctx, conn, expID)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}

	if status.Status != "analysis" {
		t.Fatalf("expected status %q, got %q", "analysis", status.Status)
	}

	if len(status.Variants) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(status.Variants))
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	t.Cleanup(func() { conn.Close() })

	return conn
}

func seedRound(t *testing.T, conn db.DBTX) string {
	t.Helper()

	convID := id.V7()
	_, err := conn.ExecContext(context.Background(),
		"INSERT INTO conversation (id, platform, external_conversation_id) VALUES (?, 'whatsapp', ?)",
		convID, "ext-"+convID)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	actID := id.V7()

	err = db.ActivationInsert(context.Background(), conn, db.ActivationRow{
		ID:             actID,
		ConversationID: convID,
		Model:          "test-model",
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed activation: %v", err)
	}

	roundID, err := db.ActivationRoundInsert(context.Background(), conn, db.ActivationRoundInsertParams{
		ActivationID: actID,
		Round:        0,
		UserInput:    "test input",
		ModelOutput:  "test output",
	})
	if err != nil {
		t.Fatalf("seed round: %v", err)
	}

	return roundID
}
