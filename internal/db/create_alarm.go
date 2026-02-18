package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func CreateAlarm(ctx context.Context, db *sql.DB, originContactID, originConversationID, goal, recurrence string, nextFireAt time.Time) (Alarm, error) {
	id := NewID()
	now := time.Now()

	contactID := any(nil)
	if originContactID != "" {
		contactID = originContactID
	}

	conversationID := any(nil)
	if originConversationID != "" {
		conversationID = originConversationID
	}

	rec := any(nil)
	if recurrence != "" {
		rec = recurrence
	}

	_, err := db.ExecContext(ctx, queries.CreateAlarm, id, contactID, conversationID, goal, rec, nextFireAt, now)
	if err != nil {
		return Alarm{}, err
	}

	return Alarm{
		ID:                   id,
		OriginContactID:      sql.NullString{String: originContactID, Valid: originContactID != ""},
		OriginConversationID: sql.NullString{String: originConversationID, Valid: originConversationID != ""},
		Goal:                 goal,
		Recurrence:           sql.NullString{String: recurrence, Valid: recurrence != ""},
		NextFireAt:           sql.NullTime{Time: nextFireAt, Valid: true},
		CreatedAt:            now,
	}, nil
}
