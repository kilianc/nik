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
	NextFireAt           time.Time
}

func CreateAlarm(ctx context.Context, db *sql.DB, p CreateAlarmParams) (Alarm, error) {
	newID := id.V7()
	now := time.Now()

	contactID := any(nil)
	if p.OriginContactID != "" {
		contactID = p.OriginContactID
	}

	rec := any(nil)
	if p.Recurrence != "" {
		rec = p.Recurrence
	}

	_, err := db.ExecContext(ctx, queries.CreateAlarm, newID, contactID, p.OriginConversationID, p.Goal, rec, p.NextFireAt, now)
	if err != nil {
		return Alarm{}, err
	}

	return Alarm{
		ID:                   newID,
		OriginContactID:      sql.NullString{String: p.OriginContactID, Valid: p.OriginContactID != ""},
		OriginConversationID: sql.NullString{String: p.OriginConversationID, Valid: true},
		Goal:                 p.Goal,
		Recurrence:           sql.NullString{String: p.Recurrence, Valid: p.Recurrence != ""},
		NextFireAt:           sql.NullTime{Time: p.NextFireAt, Valid: true},
		CreatedAt:            now,
	}, nil
}

func scanAlarm(s scanner) (Alarm, error) {
	var a Alarm
	err := s.Scan(
		&a.ID,
		&a.OriginContactID,
		&a.OriginConversationID,
		&a.Goal,
		&a.Recurrence,
		&a.NextFireAt,
		&a.LastFiredAt,
		&a.CreatedAt,
	)
	return a, err
}

func DueAlarms(ctx context.Context, db *sql.DB, now time.Time) ([]Alarm, error) {
	rows, err := db.QueryContext(ctx, queries.DueAlarms, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []Alarm
	for rows.Next() {
		a, scanErr := scanAlarm(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		alarms = append(alarms, a)
	}

	return alarms, rows.Err()
}

type AlarmUpdateParams struct {
	Goal       *string
	Recurrence *string
	NextFireAt any
}

func AlarmUpdate(ctx context.Context, db *sql.DB, id string, p AlarmUpdateParams) error {
	_, err := db.ExecContext(ctx, queries.AlarmUpdate, id, p.Goal, p.Recurrence, p.NextFireAt)
	return err
}

func AlarmCancel(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, queries.AlarmCancel, id, time.Now())
	return err
}

func AlarmListCreated(ctx context.Context, db *sql.DB, conversationID string, since time.Time) ([]Alarm, error) {
	rows, err := db.QueryContext(ctx, queries.AlarmListCreated, conversationID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []Alarm
	for rows.Next() {
		a, scanErr := scanAlarm(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		alarms = append(alarms, a)
	}

	return alarms, rows.Err()
}

func AlarmClaim(ctx context.Context, db *sql.DB, id string, now time.Time) error {
	_, err := db.ExecContext(ctx, queries.AlarmClaim, id, now)
	return err
}
