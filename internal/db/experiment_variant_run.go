package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func ExperimentVariantRunInsert(ctx context.Context, db DBTX, variantID string) (ExperimentVariantRun, error) {
	runID := id.V7()

	_, err := db.ExecContext(ctx, queries.ExperimentVariantRunInsert, runID, variantID)
	if err != nil {
		return ExperimentVariantRun{}, fmt.Errorf("insert experiment_variant_run for variant %s: %w", variantID, err)
	}

	return ExperimentVariantRun{
		ID:                  runID,
		ExperimentVariantID: variantID,
		ToolCalls:           "[]",
		ReasoningSummaries:  "[]",
		CreatedAt:           time.Now().UTC(),
	}, nil
}

func ExperimentVariantRunGet(ctx context.Context, conn *sql.DB, runID string) (ExperimentVariantRun, error) {
	var r ExperimentVariantRun
	var isDesired sql.NullInt64
	var effort, verbosity sql.NullString

	err := conn.QueryRowContext(ctx, queries.ExperimentVariantRunGet, runID).Scan(
		&r.ID,
		&r.ExperimentVariantID,
		&r.ToolCalls,
		&r.ModelOutput,
		&r.ReasoningSummaries,
		&isDesired,
		&r.Rationale,
		&r.InputTokens,
		&r.OutputTokens,
		&r.CachedTokens,
		&r.ReasoningTokens,
		&r.CreatedAt,
		&r.Model,
		&r.Instructions,
		&r.ToolSchemas,
		&r.Messages,
		&effort,
		&verbosity,
		&r.Patches,
	)
	if err != nil {
		return ExperimentVariantRun{}, fmt.Errorf("get experiment_variant_run %s: %w", runID, err)
	}

	if isDesired.Valid {
		v := isDesired.Int64 != 0
		r.IsDesired = &v
	}

	r.ReasoningEffort = effort.String
	r.Verbosity = verbosity.String

	return r, nil
}

func ExperimentVariantRunSaveResult(ctx context.Context, db DBTX, run ExperimentVariantRun) error {
	_, err := db.ExecContext(ctx, queries.ExperimentVariantRunSaveResult,
		run.ID,
		run.ToolCalls,
		run.ModelOutput,
		run.ReasoningSummaries,
		run.InputTokens,
		run.OutputTokens,
		run.CachedTokens,
		run.ReasoningTokens,
	)
	if err != nil {
		return fmt.Errorf("save result for experiment_variant_run %s: %w", run.ID, err)
	}

	return nil
}

func ExperimentVariantRunUpdate(ctx context.Context, db DBTX, runID string, isDesired bool, rationale string) (string, error) {
	var desired int
	if isDesired {
		desired = 1
	}

	_, err := db.ExecContext(ctx, queries.ExperimentVariantRunUpdate, runID, desired, rationale)
	if err != nil {
		return "", fmt.Errorf("update experiment_variant_run %s: %w", runID, err)
	}

	var variantID string
	err = db.QueryRowContext(ctx,
		"SELECT experiment_variant_id FROM experiment_variant_run WHERE id = ?1", runID,
	).Scan(&variantID)
	if err != nil {
		return "", fmt.Errorf("get variant for run %s: %w", runID, err)
	}

	return variantID, nil
}

func ExperimentVariantRunList(ctx context.Context, db DBTX, variantID string) ([]ExperimentVariantRun, error) {
	rows, err := db.QueryContext(ctx, queries.ExperimentVariantRunList, variantID)
	if err != nil {
		return nil, fmt.Errorf("list experiment_variant_runs for variant %s: %w", variantID, err)
	}
	defer rows.Close()

	var runs []ExperimentVariantRun

	for rows.Next() {
		var r ExperimentVariantRun
		var isDesired sql.NullInt64

		err = rows.Scan(
			&r.ID,
			&r.ExperimentVariantID,
			&r.ToolCalls,
			&r.ModelOutput,
			&r.ReasoningSummaries,
			&isDesired,
			&r.Rationale,
			&r.InputTokens,
			&r.OutputTokens,
			&r.CachedTokens,
			&r.ReasoningTokens,
			&r.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan experiment_variant_run: %w", err)
		}

		if isDesired.Valid {
			v := isDesired.Int64 != 0
			r.IsDesired = &v
		}

		runs = append(runs, r)
	}

	return runs, rows.Err()
}
