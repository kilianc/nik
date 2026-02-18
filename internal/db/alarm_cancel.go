package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func AlarmCancel(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, queries.AlarmCancel, id, time.Now())
	return err
}
