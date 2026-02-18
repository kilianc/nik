package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

func AlarmOccurrenceUpdateNote(ctx context.Context, db *sql.DB, id, note string) error {
	_, err := db.ExecContext(ctx, queries.AlarmOccurrenceUpdateNote, id, note)
	return err
}
