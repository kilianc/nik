package db

import (
	"context"
	"database/sql"
	"fmt"
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

	_, err := db.ExecContext(ctx, queries.AlarmInsert, newID, contactID, p.OriginConversationID, p.Goal, rec, p.NextFireAt, now)
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
	rows, err := db.QueryContext(ctx, queries.AlarmDue, now)
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
	Goal        *string
	Recurrence  *string
	NextFireAt  any
	LastFiredAt any
}

func AlarmUpdate(ctx context.Context, db *sql.DB, id string, p AlarmUpdateParams) error {
	_, err := db.ExecContext(ctx, queries.AlarmUpdate, id, p.Goal, p.Recurrence, p.NextFireAt, p.LastFiredAt)
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

// AlarmGet looks up a single active alarm by ID or goal prefix.
func AlarmGet(ctx context.Context, db *sql.DB, identifier string) (Alarm, bool, error) {
	row := db.QueryRowContext(ctx, queries.AlarmGet, identifier)

	a, err := scanAlarm(row)
	if err == sql.ErrNoRows {
		return Alarm{}, false, nil
	}
	if err != nil {
		return Alarm{}, false, err
	}

	return a, true, nil
}

func StaleRecurringAlarms(ctx context.Context, db *sql.DB, now time.Time) ([]Alarm, error) {
	rows, err := db.QueryContext(ctx, queries.AlarmStaleRecurring, now)
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

func AlarmFire(ctx context.Context, conn *sql.DB, alarmID string, now time.Time) (AlarmOccurrence, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return AlarmOccurrence{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	occID := id.V7()

	_, err = tx.ExecContext(ctx, queries.AlarmOccurrenceInsert, occID, alarmID, now)
	if err != nil {
		return AlarmOccurrence{}, fmt.Errorf("insert occurrence: %w", err)
	}

	_, err = tx.ExecContext(ctx, queries.AlarmUpdate, alarmID, nil, nil, nil, now)
	if err != nil {
		return AlarmOccurrence{}, fmt.Errorf("set alarm fired: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return AlarmOccurrence{}, fmt.Errorf("commit: %w", err)
	}

	return AlarmOccurrence{
		ID:      occID,
		AlarmID: alarmID,
		FiredAt: now,
	}, nil
}
