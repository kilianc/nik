package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type CreateAlarmParams struct {
	OriginContactID      string
	OriginConversationID string
	Goal                 string
	Recurrence           string
	Source               string
	SourceID             string
	NextFireAt           time.Time
}

func CreateAlarm(ctx context.Context, db *sql.DB, p CreateAlarmParams) (Alarm, error) {
	newID := id.V7()
	now := time.Now()

	contactID := any(nil)
	if p.OriginContactID != "" {
		contactID = p.OriginContactID
	}

	conversationID := any(nil)
	if p.OriginConversationID != "" {
		conversationID = p.OriginConversationID
	}

	rec := any(nil)
	if p.Recurrence != "" {
		rec = p.Recurrence
	}

	src := any(nil)
	if p.Source != "" {
		src = p.Source
	}

	srcID := any(nil)
	if p.SourceID != "" {
		srcID = p.SourceID
	}

	_, err := db.ExecContext(ctx, queries.CreateAlarm, newID, contactID, conversationID, p.Goal, rec, src, srcID, p.NextFireAt, now)
	if err != nil {
		return Alarm{}, err
	}

	return Alarm{
		ID:                   newID,
		OriginContactID:      sql.NullString{String: p.OriginContactID, Valid: p.OriginContactID != ""},
		OriginConversationID: sql.NullString{String: p.OriginConversationID, Valid: p.OriginConversationID != ""},
		Goal:                 p.Goal,
		Recurrence:           sql.NullString{String: p.Recurrence, Valid: p.Recurrence != ""},
		Source:               sql.NullString{String: p.Source, Valid: p.Source != ""},
		SourceID:             sql.NullString{String: p.SourceID, Valid: p.SourceID != ""},
		NextFireAt:           sql.NullTime{Time: p.NextFireAt, Valid: true},
		CreatedAt:            now,
	}, nil
}
