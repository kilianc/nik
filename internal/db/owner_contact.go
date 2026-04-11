package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

const OwnerContactID = "00000000-0000-0000-0000-000000000002"

func OwnerContactEnsure(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, queries.ContactOwnerEnsure, OwnerContactID)
	if err != nil {
		return err
	}

	return nil
}
