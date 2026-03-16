package db

import (
	"context"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func AlarmOccurrenceInsert(ctx context.Context, db DBTX, alarmID string, firedAt time.Time) (AlarmOccurrence, error) {
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

func AlarmOccurrenceUpdateLatestNote(ctx context.Context, db DBTX, alarmID, note string) (bool, error) {
	result, err := db.ExecContext(ctx, queries.AlarmOccurrenceUpdate, alarmID, note)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows > 0, nil
}
