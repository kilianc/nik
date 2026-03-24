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

	expID, err := CreateExperiment(ctx, conn, roundID, "model should call message_noop")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	notes := "The model re-acknowledged because ### New contained system events."
	err = db.ExperimentUpdate(ctx, conn, db.ExperimentUpdateParams{
		ID:    expID,
		Notes: &notes,
	})
	if err != nil {
		t.Fatalf("update notes: %v", err)
	}

	report, err := RenderReport(ctx, conn, expID)
	if err != nil {
		t.Fatalf("render report: %v", err)
	}

	if !strings.Contains(report, "# Experiment") {
		t.Fatal("expected report to contain header")
	}
	if !strings.Contains(report, "## Target") {
		t.Fatal("expected report to contain Target section")
	}
	if !strings.Contains(report, "## Desired Outcome") {
		t.Fatal("expected report to contain Desired Outcome section")
	}
	if !strings.Contains(report, "model should call message_noop") {
		t.Fatal("expected report to contain desired outcome text")
	}
	if !strings.Contains(report, "## Trace") {
		t.Fatal("expected report to contain Trace section")
	}
	if !strings.Contains(report, "### New contained system events") {
		t.Fatal("expected report to contain trace text")
	}
	if !strings.Contains(report, "Status: analysis") {
		t.Fatal("expected report to show analysis status")
	}
}

func TestRenderReportWithRuns(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	variants, err := db.ExperimentVariantList(ctx, conn, expID)
	if err != nil {
		t.Fatalf("list variants: %v", err)
	}

	baselineID := variants[0].ID

	_, err = RecordRun(ctx, conn, baselineID, ReplayResult{
		ToolCalls:    []ToolCall{{Name: "message_noop"}},
		InputTokens:  4521,
		OutputTokens: 89,
	}, true)
	if err != nil {
		t.Fatalf("record run: %v", err)
	}

	_, err = RecordRun(ctx, conn, baselineID, ReplayResult{
		ToolCalls:    []ToolCall{{Name: "task_spawn"}},
		ModelOutput:  "On it.",
		InputTokens:  4521,
		OutputTokens: 234,
	}, false)
	if err != nil {
		t.Fatalf("record run: %v", err)
	}

	report, err := RenderReport(ctx, conn, expID)
	if err != nil {
		t.Fatalf("render report: %v", err)
	}

	if !strings.Contains(report, "## Baseline") {
		t.Fatal("expected report to contain Baseline section")
	}
	if !strings.Contains(report, "1 hit, 1 miss") {
		t.Fatal("expected report to show hit/miss counts")
	}
	if !strings.Contains(report, "message_noop") {
		t.Fatal("expected report to contain tool call name")
	}

	if strings.Contains(report, "## Comparison") {
		t.Fatal("should not show Comparison with only 1 variant having runs")
	}
}

func TestRenderReportComparison(t *testing.T) {
	ctx := context.Background()
	conn := openTestDB(t)
	roundID := seedRound(t, conn)

	expID, err := CreateExperiment(ctx, conn, roundID, "desired")
	if err != nil {
		t.Fatalf("create experiment: %v", err)
	}

	variants, err := db.ExperimentVariantList(ctx, conn, expID)
	if err != nil {
		t.Fatalf("list variants: %v", err)
	}

	baselineID := variants[0].ID

	_, err = RecordRun(ctx, conn, baselineID, ReplayResult{
		ToolCalls:   []ToolCall{{Name: "message_noop"}},
		InputTokens: 4521, OutputTokens: 89,
	}, true)
	if err != nil {
		t.Fatalf("record baseline run: %v", err)
	}

	varID, err := CreateVariant(ctx, conn, expID, "shorter-ack", "hypothesis", []Patch{{File: "brain.md", Old: "old", New: "new"}}, "", "")
	if err != nil {
		t.Fatalf("create variant: %v", err)
	}

	_, err = RecordRun(ctx, conn, varID, ReplayResult{
		ToolCalls:   []ToolCall{{Name: "message_noop"}},
		InputTokens: 4600, OutputTokens: 91,
	}, true)
	if err != nil {
		t.Fatalf("record variant run: %v", err)
	}

	report, err := RenderReport(ctx, conn, expID)
	if err != nil {
		t.Fatalf("render report: %v", err)
	}

	if !strings.Contains(report, "## Comparison") {
		t.Fatal("expected report to contain Comparison section")
	}
	if !strings.Contains(report, "shorter-ack") {
		t.Fatal("expected report to contain variant name in comparison")
	}
}
