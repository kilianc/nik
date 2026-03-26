package db

import (
	"context"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func ExperimentVariantInsert(ctx context.Context, db DBTX, p ExperimentVariantInsertParams) error {
	_, err := db.ExecContext(ctx, queries.ExperimentVariantInsert,
		p.ID,
		p.ExperimentID,
		p.Name,
		p.Hypothesis,
		p.Patches,
		p.ReasoningEffort,
		p.Verbosity,
	)
	if err != nil {
		return fmt.Errorf("insert experiment_variant %s: %w", p.ID, err)
	}

	return nil
}

func ExperimentVariantGet(ctx context.Context, db DBTX, idOrShort string) (ExperimentVariant, error) {
	var v ExperimentVariant

	err := db.QueryRowContext(ctx, queries.ExperimentVariantGet, idOrShort).Scan(
		&v.ID,
		&v.ExperimentID,
		&v.Name,
		&v.Hypothesis,
		&v.Patches,
		&v.ReasoningEffort,
		&v.Verbosity,
		&v.RunCount,
		&v.DesiredCount,
		&v.CreatedAt,
		&v.UpdatedAt,
	)
	if err != nil {
		return v, fmt.Errorf("get experiment_variant %s: %w", idOrShort, err)
	}

	return v, nil
}

func ExperimentVariantList(ctx context.Context, db DBTX, experimentID string) ([]ExperimentVariant, error) {
	rows, err := db.QueryContext(ctx, queries.ExperimentVariantList, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list experiment_variants for %s: %w", experimentID, err)
	}
	defer rows.Close()

	var variants []ExperimentVariant

	for rows.Next() {
		var v ExperimentVariant

		err = rows.Scan(
			&v.ID,
			&v.ExperimentID,
			&v.Name,
			&v.Hypothesis,
			&v.Patches,
			&v.ReasoningEffort,
			&v.Verbosity,
			&v.RunCount,
			&v.DesiredCount,
			&v.CreatedAt,
			&v.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan experiment_variant: %w", err)
		}

		variants = append(variants, v)
	}

	return variants, rows.Err()
}

func ExperimentVariantRefreshCounts(ctx context.Context, db DBTX, variantID string) error {
	_, err := db.ExecContext(ctx, queries.ExperimentVariantRefreshCounts, variantID)
	if err != nil {
		return fmt.Errorf("refresh counts for experiment_variant %s: %w", variantID, err)
	}

	return nil
}

func ExperimentVariantUpdate(ctx context.Context, db DBTX, p ExperimentVariantUpdateParams) error {
	_, err := db.ExecContext(ctx, queries.ExperimentVariantUpdate,
		p.ID,
		p.RunCount,
		p.DesiredCount,
	)
	if err != nil {
		return fmt.Errorf("update experiment_variant %s: %w", p.ID, err)
	}

	return nil
}
