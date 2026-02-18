package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func AlarmClaim(ctx context.Context, db *sql.DB, id string, now time.Time) error {
	_, err := db.ExecContext(ctx, queries.AlarmClaim, id, now)
	return err
}
