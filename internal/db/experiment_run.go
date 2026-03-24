package db

import (
	"context"
	"fmt"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func ExperimentRunInsert(ctx context.Context, db DBTX, p ExperimentRunInsertParams) (string, error) {
	runID := id.V7()

	var isDesired int
	if p.IsDesired {
		isDesired = 1
	}

	_, err := db.ExecContext(ctx, queries.ExperimentRunInsert,
		runID,
		p.ExperimentVariantID,
		p.ToolCalls,
		p.ModelOutput,
		p.ReasoningSummaries,
		isDesired,
		p.InputTokens,
		p.OutputTokens,
		p.CachedTokens,
		p.ReasoningTokens,
	)
	if err != nil {
		return "", fmt.Errorf("insert experiment_run for variant %s: %w", p.ExperimentVariantID, err)
	}

	return runID, nil
}

func ExperimentRunList(ctx context.Context, db DBTX, variantID string) ([]ExperimentRun, error) {
	rows, err := db.QueryContext(ctx, queries.ExperimentRunList, variantID)
	if err != nil {
		return nil, fmt.Errorf("list experiment_runs for variant %s: %w", variantID, err)
	}
	defer rows.Close()

	var runs []ExperimentRun

	for rows.Next() {
		var r ExperimentRun

		err = rows.Scan(
			&r.ID,
			&r.ExperimentVariantID,
			&r.ToolCalls,
			&r.ModelOutput,
			&r.ReasoningSummaries,
			&r.IsDesired,
			&r.InputTokens,
			&r.OutputTokens,
			&r.CachedTokens,
			&r.ReasoningTokens,
			&r.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan experiment_run: %w", err)
		}

		runs = append(runs, r)
	}

	return runs, rows.Err()
}
