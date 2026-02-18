package db

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/queries"
)

func AlarmOccurrenceSummary(ctx context.Context, db *sql.DB, alarmID string, limit int) ([]AlarmOccurrence, error) {
	rows, err := db.QueryContext(ctx, queries.AlarmOccurrenceSummary, alarmID, limit)
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
			&o.NextFireAtSet,
			&o.FiredAt,
		)
		if err != nil {
			return nil, err
		}
		occurrences = append(occurrences, o)
	}

	return occurrences, rows.Err()
}
