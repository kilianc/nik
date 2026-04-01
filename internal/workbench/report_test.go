package workbench

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/db"
)

func TestRenderReportBasicSections(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "model should call done", "initial notes")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	analysis := "The model re-acknowledged because ### New contained system events."
	err = db.ExperimentUpdate(ctx, conn, db.ExperimentUpdateParams{
		ID:       expID,
		Analysis: &analysis,
	})
	if err != nil {
		t.Fatalf("update analysis: %v", err)
	}

	report, err := RenderReport(ctx, conn, expID)
	if err != nil {
		t.Fatalf("render report: %v", err)
	}

	checks := []struct {
		label string
		text  string
	}{
		{"header", "# Experiment"},
		{"target section", "## Target"},
		{"desired outcome section", "## Desired Outcome"},
		{"desired outcome text", "model should call done"},
		{"analysis section", "## Analysis"},
		{"analysis text", "### New contained system events"},
		{"status", "**Status:** analysis"},
		{"variants section", "## Variants"},
		{"baseline detail", "v0 — baseline"},
	}

	for _, c := range checks {
		if !strings.Contains(report, c.text) {
			t.Fatalf("expected report to contain %s (%q), got:\n%s", c.label, c.text, report)
		}
	}
}

func TestRenderReportWithRuns(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired", "analysis")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	variants, err := db.ExperimentVariantList(ctx, conn, expID)
	if err != nil {
		t.Fatalf("list variants: %v", err)
	}

	baselineID := variants[0].ID

	run1, err := db.ExperimentVariantRunInsert(ctx, conn, baselineID)
	if err != nil {
		t.Fatalf("insert run 1: %v", err)
	}

	run1.ToolCalls = `[{"name":"done"}]`
	run1.InputTokens = 4521
	run1.OutputTokens = 89

	err = db.ExperimentVariantRunSaveResult(ctx, conn, run1)
	if err != nil {
		t.Fatalf("save run 1: %v", err)
	}

	run2, err := db.ExperimentVariantRunInsert(ctx, conn, baselineID)
	if err != nil {
		t.Fatalf("insert run 2: %v", err)
	}

	run2.ToolCalls = `[{"name":"task_spawn"}]`
	run2.ModelOutput = "On it."
	run2.InputTokens = 4521
	run2.OutputTokens = 234

	err = db.ExperimentVariantRunSaveResult(ctx, conn, run2)
	if err != nil {
		t.Fatalf("save run 2: %v", err)
	}

	err = db.ExperimentVariantRefreshCounts(ctx, conn, baselineID)
	if err != nil {
		t.Fatalf("refresh counts: %v", err)
	}

	report, err := RenderReport(ctx, conn, expID)
	if err != nil {
		t.Fatalf("render report: %v", err)
	}

	if !strings.Contains(report, "v0 — baseline") {
		t.Fatal("expected report to contain v0 baseline detail section")
	}
	if !strings.Contains(report, "2 runs, N/A") {
		t.Fatal("expected unclassified runs to show N/A")
	}
	if !strings.Contains(report, "done") {
		t.Fatal("expected report to contain tool call name")
	}
	if !strings.Contains(report, "Rationale") {
		t.Fatal("expected run table to contain Rationale column")
	}
}

func TestRenderReportVariantTable(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired", "analysis")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	variants, err := db.ExperimentVariantList(ctx, conn, expID)
	if err != nil {
		t.Fatalf("list variants: %v", err)
	}

	baselineID := variants[0].ID

	bRun, err := db.ExperimentVariantRunInsert(ctx, conn, baselineID)
	if err != nil {
		t.Fatalf("insert baseline run: %v", err)
	}

	bRun.ToolCalls = `[{"name":"done"}]`
	bRun.InputTokens = 4521
	bRun.OutputTokens = 89

	err = db.ExperimentVariantRunSaveResult(ctx, conn, bRun)
	if err != nil {
		t.Fatalf("save baseline run: %v", err)
	}

	err = db.ExperimentVariantRefreshCounts(ctx, conn, baselineID)
	if err != nil {
		t.Fatalf("refresh baseline counts: %v", err)
	}

	varID, err := CreateExperimentVariant(ctx, conn, expID, "shorter-ack", "hypothesis", "--- a/instructions\n+++ b/instructions\n@@ -1,1 +1,1 @@\n-old\n+new\n", "", "")
	if err != nil {
		t.Fatalf("create variant: %v", err)
	}

	vRun, err := db.ExperimentVariantRunInsert(ctx, conn, varID)
	if err != nil {
		t.Fatalf("insert variant run: %v", err)
	}

	vRun.ToolCalls = `[{"name":"done"}]`
	vRun.InputTokens = 4600
	vRun.OutputTokens = 91

	err = db.ExperimentVariantRunSaveResult(ctx, conn, vRun)
	if err != nil {
		t.Fatalf("save variant run: %v", err)
	}

	err = db.ExperimentVariantRefreshCounts(ctx, conn, varID)
	if err != nil {
		t.Fatalf("refresh variant counts: %v", err)
	}

	report, err := RenderReport(ctx, conn, expID)
	if err != nil {
		t.Fatalf("render report: %v", err)
	}

	if !strings.Contains(report, "| v0 | N/A | baseline") {
		t.Fatalf("expected variant table to contain v0 baseline with N/A, got:\n%s", report)
	}
	if !strings.Contains(report, "| v1 | N/A | shorter-ack") {
		t.Fatalf("expected variant table to contain v1 shorter-ack with N/A, got:\n%s", report)
	}
	if !strings.Contains(report, "v1 — shorter-ack") {
		t.Fatalf("expected variant detail with v1 numbering, got:\n%s", report)
	}
	if !strings.Contains(report, "**Why:** hypothesis") {
		t.Fatalf("expected variant detail to show hypothesis, got:\n%s", report)
	}
}
