package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

type AlarmUpdateParams struct {
	Goal       *string
	Recurrence *string
	NextFireAt any
}

func AlarmUpdate(ctx context.Context, db *sql.DB, id string, p AlarmUpdateParams) error {
	_, err := db.ExecContext(ctx, queries.AlarmUpdate, id, p.Goal, p.Recurrence, p.NextFireAt)
	return err
}
