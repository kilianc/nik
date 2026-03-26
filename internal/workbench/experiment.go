package workbench

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

func CreateExperiment(ctx context.Context, conn *sql.DB, activationRoundID, desiredOutcome, analysis string) (string, error) {
	expID := id.V7()

	err := db.ExperimentInsert(ctx, conn, db.ExperimentInsertParams{
		ID:                expID,
		ActivationRoundID: activationRoundID,
		Status:            "analysis",
		DesiredOutcome:    desiredOutcome,
		Analysis:          analysis,
	})
	if err != nil {
		return "", err
	}

	baselineID := id.V7()

	err = db.ExperimentVariantInsert(ctx, conn, db.ExperimentVariantInsertParams{
		ID:           baselineID,
		ExperimentID: expID,
		Name:         "baseline",
		Patches:      "",
	})
	if err != nil {
		return "", fmt.Errorf("create baseline variant: %w", err)
	}

	return expID, nil
}

func UpdateExperiment(ctx context.Context, conn *sql.DB, experimentID string, status, desired, analysis *string) error {
	return db.ExperimentUpdate(ctx, conn, db.ExperimentUpdateParams{
		ID:             experimentID,
		Status:         status,
		DesiredOutcome: desired,
		Analysis:       analysis,
	})
}

func CreateExperimentVariant(ctx context.Context, conn *sql.DB, experimentID, name, hypothesis, patches, effort, verbosity string) (string, error) {
	if patches != "" {
		_, err := ParseDiff(patches)
		if err != nil {
			return "", fmt.Errorf("invalid patch: %w", err)
		}
	}

	variantID := id.V7()

	err := db.ExperimentVariantInsert(ctx, conn, db.ExperimentVariantInsertParams{
		ID:              variantID,
		ExperimentID:    experimentID,
		Name:            name,
		Hypothesis:      hypothesis,
		Patches:         patches,
		ReasoningEffort: effort,
		Verbosity:       verbosity,
	})
	if err != nil {
		return "", err
	}

	return variantID, nil
}

func CreateExperimentVariantRun(ctx context.Context, conn *sql.DB, variantID string, n, maxRounds int, clientOpts []llm.ClientOption) ([]db.ExperimentVariantRun, error) {
	if maxRounds < 1 {
		maxRounds = 5
	}

	var runs []db.ExperimentVariantRun

	for range n {
		bare, err := db.ExperimentVariantRunInsert(ctx, conn, variantID)
		if err != nil {
			return nil, fmt.Errorf("insert run: %w", err)
		}

		run, err := db.ExperimentVariantRunGet(ctx, conn, bare.ID)
		if err != nil {
			return nil, fmt.Errorf("load run context: %w", err)
		}

		err = ApplyPatches(&run)
		if err != nil {
			return nil, fmt.Errorf("apply patches: %w", err)
		}

		run, err = Run(ctx, run, maxRounds, clientOpts)
		if err != nil {
			return nil, err
		}

		err = db.ExperimentVariantRunSaveResult(ctx, conn, run)
		if err != nil {
			return nil, fmt.Errorf("save run result: %w", err)
		}

		err = db.ExperimentVariantRefreshCounts(ctx, conn, variantID)
		if err != nil {
			return nil, fmt.Errorf("refresh variant counts: %w", err)
		}

		runs = append(runs, run)
	}

	return runs, nil
}

func UpdateExperimentVariantRun(ctx context.Context, conn *sql.DB, runID string, isDesired bool, rationale string) (string, error) {
	variantID, err := db.ExperimentVariantRunUpdate(ctx, conn, runID, isDesired, rationale)
	if err != nil {
		return "", err
	}

	err = db.ExperimentVariantRefreshCounts(ctx, conn, variantID)
	if err != nil {
		return "", fmt.Errorf("refresh variant counts: %w", err)
	}

	v, err := db.ExperimentVariantGet(ctx, conn, variantID)
	if err != nil {
		return "", fmt.Errorf("get variant for experiment ID: %w", err)
	}

	return v.ExperimentID, nil
}
