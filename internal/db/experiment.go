package db

import (
	"context"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func ExperimentInsert(ctx context.Context, db DBTX, p ExperimentInsertParams) error {
	_, err := db.ExecContext(ctx, queries.ExperimentInsert,
		p.ID,
		p.ActivationRoundID,
		p.Status,
		p.DesiredOutcome,
		p.Notes,
	)
	if err != nil {
		return fmt.Errorf("insert experiment %s: %w", p.ID, err)
	}

	return nil
}

func ExperimentGet(ctx context.Context, db DBTX, idOrShort string) (Experiment, error) {
	var e Experiment

	err := db.QueryRowContext(ctx, queries.ExperimentGet, idOrShort).Scan(
		&e.ID,
		&e.ActivationRoundID,
		&e.Status,
		&e.DesiredOutcome,
		&e.Notes,
		&e.CreatedAt,
		&e.UpdatedAt,
	)
	if err != nil {
		return e, fmt.Errorf("get experiment %s: %w", idOrShort, err)
	}

	return e, nil
}

func ExperimentUpdate(ctx context.Context, db DBTX, p ExperimentUpdateParams) error {
	_, err := db.ExecContext(ctx, queries.ExperimentUpdate,
		p.ID,
		p.Status,
		p.DesiredOutcome,
		p.Notes,
	)
	if err != nil {
		return fmt.Errorf("update experiment %s: %w", p.ID, err)
	}

	return nil
}
