package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

const SystemContactID = "00000000-0000-0000-0000-000000000001"

func EnsureSystemContact(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, queries.SystemContactEnsure, SystemContactID)
	if err != nil {
		return err
	}

	return nil
}
