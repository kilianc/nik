package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func AlarmOccurrenceInsert(ctx context.Context, db *sql.DB, alarmID string, firedAt time.Time) (AlarmOccurrence, error) {
	newID := id.V7()

	_, err := db.ExecContext(ctx, queries.AlarmOccurrenceInsert, newID, alarmID, firedAt)
	if err != nil {
		return AlarmOccurrence{}, err
	}

	return AlarmOccurrence{
		ID:      newID,
		AlarmID: alarmID,
		FiredAt: firedAt,
	}, nil
}
