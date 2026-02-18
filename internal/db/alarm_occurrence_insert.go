package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func AlarmOccurrenceInsert(ctx context.Context, db *sql.DB, alarmID string, firedAt time.Time) (AlarmOccurrence, error) {
	id := NewID()

	_, err := db.ExecContext(ctx, queries.AlarmOccurrenceInsert, id, alarmID, firedAt)
	if err != nil {
		return AlarmOccurrence{}, err
	}

	return AlarmOccurrence{
		ID:      id,
		AlarmID: alarmID,
		FiredAt: firedAt,
	}, nil
}
