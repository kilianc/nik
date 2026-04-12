package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type AlarmCreateParams struct {
	OriginContactID      string
	OriginConversationID string
	Goal                 string
	Recurrence           string
	NextFireAt           time.Time
}

func AlarmCreate(ctx context.Context, db DBTX, p AlarmCreateParams) (Alarm, error) {
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
		&a.LastOccurrenceNote,
		&a.NextFireAt,
		&a.LastFiredAt,
		&a.CreatedAt,
	)
	return a, err
}

func AlarmListDue(ctx context.Context, db *sql.DB, now time.Time) ([]Alarm, error) {
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
	Goal                    *string
	Recurrence              *string
	NextFireAt              any
	LastFiredAt             any
	ApplyLastOccurrenceNote bool
	LastOccurrenceNote      any
	Cancel                  bool
}

func AlarmUpdate(ctx context.Context, db DBTX, id string, p AlarmUpdateParams) error {
	_, err := db.ExecContext(ctx, queries.AlarmUpdate,
		id,
		p.Goal,
		p.Recurrence,
		p.NextFireAt,
		p.LastFiredAt,
		p.ApplyLastOccurrenceNote,
		p.LastOccurrenceNote,
		p.Cancel,
	)
	return err
}

func AlarmCancel(ctx context.Context, db DBTX, id string) error {
	return AlarmUpdate(ctx, db, id, AlarmUpdateParams{Cancel: true})
}

// AlarmGet looks up a single active alarm by ID or goal prefix.
func AlarmGet(ctx context.Context, db DBTX, identifier string) (Alarm, bool, error) {
	row := db.QueryRowContext(ctx, queries.AlarmGet, identifier)

	a, err := scanAlarm(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Alarm{}, false, nil
	}
	if err != nil {
		return Alarm{}, false, err
	}

	return a, true, nil
}

func AlarmListStale(ctx context.Context, db *sql.DB, now time.Time) ([]Alarm, error) {
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

func AlarmFire(ctx context.Context, conn *sql.DB, a Alarm, now time.Time) (AlarmOccurrence, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return AlarmOccurrence{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	occ, err := AlarmOccurrenceInsert(ctx, tx, a.ID, now)
	if err != nil {
		return AlarmOccurrence{}, fmt.Errorf("insert occurrence: %w", err)
	}

	err = AlarmUpdate(ctx, tx, a.ID, AlarmUpdateParams{
		LastFiredAt:             now,
		ApplyLastOccurrenceNote: true,
		LastOccurrenceNote:      nil,
	})
	if err != nil {
		return AlarmOccurrence{}, fmt.Errorf("set alarm fired: %w", err)
	}

	a.LastFiredAt = sql.NullTime{Time: now, Valid: true}
	a.LastOccurrenceNote = sql.NullString{}

	if a.OriginConversationID.Valid {
		_, err = SystemMessageInsert(ctx, tx, SystemMessageParams{
			ConversationID: a.OriginConversationID.String,
			Kind:           "alarm_fired",
			Body:           a,
			SentAt:         now,
		})
		if err != nil {
			return AlarmOccurrence{}, fmt.Errorf("insert system message: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return AlarmOccurrence{}, fmt.Errorf("commit: %w", err)
	}

	return occ, nil
}
