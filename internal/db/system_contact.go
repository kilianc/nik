package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

const SystemContactID = "00000000-0000-0000-0000-000000000001"

func SystemContactEnsure(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, queries.ContactSystemEnsure, SystemContactID)
	if err != nil {
		return err
	}

	return nil
}
