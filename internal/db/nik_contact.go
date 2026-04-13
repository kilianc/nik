package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

const NikContactID = "00000000-0000-0000-0000-000000000002"

func NikContactEnsure(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, queries.ContactNikEnsure, NikContactID)
	return err
}
