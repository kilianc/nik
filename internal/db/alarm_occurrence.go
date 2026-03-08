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

func AlarmOccurrenceList(ctx context.Context, db *sql.DB, conversationID string, since time.Time) ([]AlarmOccurrence, error) {
	rows, err := db.QueryContext(ctx, queries.AlarmOccurrenceList, conversationID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var occurrences []AlarmOccurrence
	for rows.Next() {
		var o AlarmOccurrence
		err = rows.Scan(
			&o.ID,
			&o.AlarmID,
			&o.Note,
			&o.FiredAt,
			&o.Goal,
			&o.Recurrence,
		)
		if err != nil {
			return nil, err
		}
		occurrences = append(occurrences, o)
	}

	return occurrences, rows.Err()
}

func AlarmOccurrenceUpdateNoteByAlarm(ctx context.Context, db *sql.DB, alarmID, note string) error {
	_, err := db.ExecContext(ctx, queries.AlarmOccurrenceUpdateNote, alarmID, note)
	return err
}
