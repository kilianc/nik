package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

func NikContactEnsure(ctx context.Context, conn *sql.DB, nikID string) error {
	_, err := conn.ExecContext(ctx, queries.ContactNikEnsure, nikID)
	return err
}
