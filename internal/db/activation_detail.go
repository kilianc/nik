package db

import (
	"context"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

type ActivationDetailParams struct {
	ActivationID       string
	Instructions       string
	UserInput          string
	Tools              []string
	ReasoningSummaries []string
}

func ActivationDetailInsert(ctx context.Context, db DBTX, p ActivationDetailParams) error {
	_, err := db.ExecContext(ctx, queries.ActivationDetailInsert,
		p.ActivationID,
		p.Instructions,
		p.UserInput,
		MarshalStringSlice(p.Tools),
		MarshalStringSlice(p.ReasoningSummaries),
	)
	if err != nil {
		return fmt.Errorf("insert activation detail %s: %w", p.ActivationID, err)
	}

	return nil
}
