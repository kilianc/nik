package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

const NikContactID = "00000000-0000-7000-8000-000000000001"

func NikContactEnsure(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, queries.ContactNikEnsure, NikContactID)
	return err
}
