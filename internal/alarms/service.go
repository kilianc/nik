package alarms

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

type Alarm = db.Alarm
type AlarmOccurrence = db.AlarmOccurrence

type Service struct {
	db *sql.DB
}

func New(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) CreateAlarm(ctx context.Context, originContactID, originConversationID, goal, recurrence, nextFireAtStr string) (*Alarm, error) {
	nextFireAt, err := time.Parse(time.RFC3339, nextFireAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse next_fire_at %q: %w", nextFireAtStr, err)
	}

	alarm, err := db.CreateAlarm(ctx, s.db, db.CreateAlarmParams{
		OriginContactID:      originContactID,
		OriginConversationID: originConversationID,
		Goal:                 goal,
		Recurrence:           recurrence,
		NextFireAt:           nextFireAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create alarm: %w", err)
	}

	return &alarm, nil
}

func (s *Service) FireDueAlarms(ctx context.Context) {
	now := time.Now()

	alarms, err := db.DueAlarms(ctx, s.db, now)
	if err != nil {
		slog.Warn("fire due alarms", "pkg", "alarms", "error", err)
		return
	}

	for _, a := range alarms {
		_, err = db.AlarmOccurrenceInsert(ctx, s.db, a.ID, now)
		if err != nil {
			slog.Warn("log alarm occurrence", "pkg", "alarms", "alarm_id", a.ID, "error", err)
			continue
		}

		err = db.AlarmClaim(ctx, s.db, a.ID, now)
		if err != nil {
			slog.Warn("claim alarm", "pkg", "alarms", "alarm_id", a.ID, "error", err)
		}

		slog.Info("alarm fired", "pkg", "alarms", "id", a.ID)
	}
}

func (s *Service) UpdateAlarm(ctx context.Context, id string, p db.AlarmUpdateParams) error {
	return db.AlarmUpdate(ctx, s.db, id, p)
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	return db.AlarmCancel(ctx, s.db, id)
}

func (s *Service) ResolveAlarmID(ctx context.Context, shortID string) (string, error) {
	return db.ResolveShortID(ctx, s.db, "alarm", shortID)
}

func (s *Service) UpdateLatestOccurrenceNote(ctx context.Context, alarmID, note string) error {
	return db.AlarmOccurrenceUpdateNoteByAlarm(ctx, s.db, alarmID, note)
}

func (s *Service) ListOccurrences(ctx context.Context, conversationID string, since time.Time) ([]AlarmOccurrence, error) {
	return db.AlarmOccurrenceList(ctx, s.db, conversationID, since)
}

func (s *Service) ListCreated(ctx context.Context, conversationID string, since time.Time) ([]Alarm, error) {
	return db.AlarmListCreated(ctx, s.db, conversationID, since)
}
