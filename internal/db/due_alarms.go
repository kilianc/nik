package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func DueAlarms(ctx context.Context, db *sql.DB, now time.Time) ([]Alarm, error) {
	rows, err := db.QueryContext(ctx, queries.DueAlarms, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []Alarm
	for rows.Next() {
		var a Alarm
		err = rows.Scan(
			&a.ID,
			&a.OriginContactID,
			&a.OriginConversationID,
			&a.Goal,
			&a.Recurrence,
			&a.Source,
			&a.SourceID,
			&a.NextFireAt,
			&a.LastFiredAt,
			&a.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		alarms = append(alarms, a)
	}

	return alarms, rows.Err()
}
