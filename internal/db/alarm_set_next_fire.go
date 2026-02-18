package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func AlarmSetNextFire(ctx context.Context, db *sql.DB, id string, nextFireAt time.Time) error {
	_, err := db.ExecContext(ctx, queries.AlarmSetNextFire, id, nextFireAt)
	return err
}
