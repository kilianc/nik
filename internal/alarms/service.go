package alarms

import (
	"context"
	"database/sql"
	"fmt"
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

func (s *Service) CreateAlarm(ctx context.Context, originContactID, originConversationID, goal, recurrence, source, sourceID, nextFireAtStr string) (*Alarm, error) {
	nextFireAt, err := time.Parse(time.RFC3339, nextFireAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse next_fire_at %q: %w", nextFireAtStr, err)
	}

	alarm, err := db.CreateAlarm(ctx, s.db, db.CreateAlarmParams{
		OriginContactID:      originContactID,
		OriginConversationID: originConversationID,
		Goal:                 goal,
		Recurrence:           recurrence,
		Source:               source,
		SourceID:             sourceID,
		NextFireAt:           nextFireAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create alarm: %w", err)
	}

	return &alarm, nil
}

func (s *Service) DueAlarms(ctx context.Context) ([]Alarm, error) {
	return db.DueAlarms(ctx, s.db, time.Now())
}

func (s *Service) ClaimAlarm(ctx context.Context, id string) error {
	return db.AlarmClaim(ctx, s.db, id, time.Now())
}

func (s *Service) LogOccurrence(ctx context.Context, alarmID string) (AlarmOccurrence, error) {
	return db.AlarmOccurrenceInsert(ctx, s.db, alarmID, time.Now())
}

func (s *Service) UpdateAlarm(ctx context.Context, id string, p db.AlarmUpdateParams) error {
	return db.AlarmUpdate(ctx, s.db, id, p)
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	return db.AlarmCancel(ctx, s.db, id)
}

func (s *Service) UpdateOccurrenceNote(ctx context.Context, occurrenceID, note string) error {
	return db.AlarmOccurrenceUpdateNote(ctx, s.db, occurrenceID, note)
}

func (s *Service) OccurrenceSummary(ctx context.Context, alarmID string, limit int) ([]AlarmOccurrence, error) {
	return db.AlarmOccurrenceSummary(ctx, s.db, alarmID, limit)
}
