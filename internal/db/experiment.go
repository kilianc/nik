package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func ExperimentInsert(ctx context.Context, db DBTX, p ExperimentInsertParams) error {
	_, err := db.ExecContext(ctx, queries.ExperimentInsert,
		p.ID,
		p.ActivationRoundID,
		p.Status,
		p.DesiredOutcome,
		p.Analysis,
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
		&e.Analysis,
		&e.CreatedAt,
		&e.UpdatedAt,
	)
	if err != nil {
		return e, fmt.Errorf("get experiment %s: %w", idOrShort, err)
	}

	return e, nil
}

func ExperimentGetFull(ctx context.Context, conn *sql.DB, idOrShort string) (Experiment, error) {
	exp, err := ExperimentGet(ctx, conn, idOrShort)
	if err != nil {
		return Experiment{}, err
	}

	exp.Round, err = ActivationRoundGet(ctx, conn, exp.ActivationRoundID)
	if err != nil {
		return Experiment{}, fmt.Errorf("load round: %w", err)
	}

	exp.Activation, err = ActivationGet(ctx, conn, exp.Round.ActivationID)
	if err != nil {
		return Experiment{}, fmt.Errorf("load activation: %w", err)
	}

	exp.ToolCalls, err = ToolCallList(ctx, conn, exp.Round.ActivationID, &exp.Round.Round)
	if err != nil {
		return Experiment{}, fmt.Errorf("load tool calls: %w", err)
	}

	exp.Variants, err = ExperimentVariantList(ctx, conn, exp.ID)
	if err != nil {
		return Experiment{}, fmt.Errorf("load variants: %w", err)
	}

	for i := range exp.Variants {
		exp.Variants[i].Runs, err = ExperimentVariantRunList(ctx, conn, exp.Variants[i].ID)
		if err != nil {
			return Experiment{}, fmt.Errorf("load runs for variant %s: %w", exp.Variants[i].ID, err)
		}
	}

	return exp, nil
}

func ExperimentUpdate(ctx context.Context, db DBTX, p ExperimentUpdateParams) error {
	_, err := db.ExecContext(ctx, queries.ExperimentUpdate,
		p.ID,
		p.Status,
		p.DesiredOutcome,
		p.Analysis,
	)
	if err != nil {
		return fmt.Errorf("update experiment %s: %w", p.ID, err)
	}

	return nil
}
