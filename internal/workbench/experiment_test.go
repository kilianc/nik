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

	expID, err := CreateExperiment(ctx, conn, roundID, "model should noop", "trace analysis")
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

	if variants[0].Patches != "" {
		t.Fatalf("expected empty patches, got %q", variants[0].Patches)
	}
}

func TestCreateExperimentVariant(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired outcome", "analysis")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	patches := "--- a/instructions\n+++ b/instructions\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	varID, err := CreateExperimentVariant(ctx, conn, expID, "shorter-ack", "reduce duplicates", patches, "medium", "low")
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
}

func TestUpdateExperimentVariantRun(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired outcome", "analysis")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	variants, err := db.ExperimentVariantList(ctx, conn, expID)
	if err != nil {
		t.Fatalf("list variants: %v", err)
	}

	baselineID := variants[0].ID

	run, err := db.ExperimentVariantRunInsert(ctx, conn, baselineID)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	run.ToolCalls = `[{"name":"message_send"}]`
	run.InputTokens = 100
	run.OutputTokens = 50

	err = db.ExperimentVariantRunSaveResult(ctx, conn, run)
	if err != nil {
		t.Fatalf("save run result: %v", err)
	}

	gotExpID, err := UpdateExperimentVariantRun(ctx, conn, run.ID, true, "correct behavior")
	if err != nil {
		t.Fatalf("update variant run: %v", err)
	}

	if gotExpID != expID {
		t.Fatalf("expected experiment ID %q, got %q", expID, gotExpID)
	}

	runs, err := db.ExperimentVariantRunList(ctx, conn, baselineID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}

	if runs[0].IsDesired == nil || !*runs[0].IsDesired {
		t.Fatal("expected run to be marked desired")
	}
	if runs[0].Rationale != "correct behavior" {
		t.Fatalf("expected rationale %q, got %q", "correct behavior", runs[0].Rationale)
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
	})
	if err != nil {
		t.Fatalf("seed round: %v", err)
	}

	return roundID
}
