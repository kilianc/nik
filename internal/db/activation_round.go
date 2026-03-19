package db

import (
	"context"
	"fmt"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type ActivationRoundInsertParams struct {
	ActivationID       string
	Round              int
	UserInput          string
	ModelOutput        string
	ReasoningSummaries []string
}

func ActivationRoundInsert(ctx context.Context, db DBTX, p ActivationRoundInsertParams) (string, error) {
	roundID := id.V7()

	_, err := db.ExecContext(ctx, queries.ActivationRoundInsert,
		roundID,
		p.ActivationID,
		p.Round,
		p.UserInput,
		p.ModelOutput,
		MarshalStringSlice(p.ReasoningSummaries),
	)
	if err != nil {
		return "", fmt.Errorf("insert activation_round %s round %d: %w", p.ActivationID, p.Round, err)
	}

	return roundID, nil
}
