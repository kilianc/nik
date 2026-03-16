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

type AlarmUpdated struct {
	Alarm db.Alarm `json:"alarm"`
	Note  string   `json:"note,omitempty"`
}

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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	alarm, err := db.CreateAlarm(ctx, tx, db.CreateAlarmParams{
		OriginContactID:      originContactID,
		OriginConversationID: originConversationID,
		Goal:                 goal,
		Recurrence:           recurrence,
		NextFireAt:           nextFireAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create alarm: %w", err)
	}

	err = db.InsertSystemMessage(ctx, tx, db.SystemMessageParams{
		ConversationID: originConversationID,
		Kind:           "alarm_created",
		Body:           alarm,
		SentAt:         alarm.CreatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("insert system message: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
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
		_, err = db.AlarmFire(ctx, s.db, a, now)
		if err != nil {
			slog.Warn("fire alarm", "pkg", "alarms", "alarm_id", a.ID, "error", err)
			continue
		}

		slog.Info("alarm fired", "pkg", "alarms", "id", a.ID)
	}
}

type UpdateParams struct {
	Goal       *string
	Recurrence *string
	NextFireAt *time.Time
	Note       string
}

func (p UpdateParams) hasAlarmFields() bool {
	return p.Goal != nil || p.Recurrence != nil || p.NextFireAt != nil
}

func (s *Service) Update(ctx context.Context, alarmID string, p UpdateParams) error {
	if !p.hasAlarmFields() && p.Note == "" {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	applyLastOccurrenceNote := false
	var lastOccurrenceNote any

	if p.Note != "" {
		updated, err := db.AlarmOccurrenceUpdateLatestNote(ctx, tx, alarmID, p.Note)
		if err != nil {
			return fmt.Errorf("update occurrence note: %w", err)
		}
		if !updated {
			return fmt.Errorf("alarm %s has no occurrence to annotate", alarmID)
		}

		applyLastOccurrenceNote = true
		lastOccurrenceNote = p.Note
	}

	var nextFireAt any
	if p.NextFireAt != nil {
		nextFireAt = *p.NextFireAt
	}

	err = db.AlarmUpdate(ctx, tx, alarmID, db.AlarmUpdateParams{
		Goal:                    p.Goal,
		Recurrence:              p.Recurrence,
		NextFireAt:              nextFireAt,
		ApplyLastOccurrenceNote: applyLastOccurrenceNote,
		LastOccurrenceNote:      lastOccurrenceNote,
	})
	if err != nil {
		return fmt.Errorf("update alarm: %w", err)
	}

	a, found, err := db.AlarmGet(ctx, tx, alarmID)
	if err != nil {
		return fmt.Errorf("get alarm for update message: %w", err)
	}
	if !found {
		return fmt.Errorf("alarm %s not found", alarmID)
	}

	if a.OriginConversationID.Valid {
		err = db.InsertSystemMessage(ctx, tx, db.SystemMessageParams{
			ConversationID: a.OriginConversationID.String,
			Kind:           "alarm_updated",
			Body:           AlarmUpdated{Alarm: a, Note: p.Note},
			SentAt:         time.Now().UTC(),
		})
		if err != nil {
			return fmt.Errorf("insert alarm_updated message: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	return db.AlarmCancel(ctx, s.db, id)
}

func (s *Service) ResolveAlarmID(ctx context.Context, shortID string) (string, error) {
	return db.ResolveShortID(ctx, s.db, "alarm", shortID)
}

func (s *Service) StaleAlarmReflex() func(ctx context.Context) {
	return func(ctx context.Context) {
		s.healStaleAlarms(ctx)
	}
}

func (s *Service) healStaleAlarms(ctx context.Context) {
	now := time.Now()

	stale, err := db.StaleRecurringAlarms(ctx, s.db, now)
	if err != nil {
		slog.Warn("find stale alarms", "pkg", "alarms", "error", err)
		return
	}

	for _, a := range stale {
		if !a.OriginConversationID.Valid {
			continue
		}

		err = db.InsertSystemMessage(ctx, s.db, db.SystemMessageParams{
			ConversationID: a.OriginConversationID.String,
			Kind:           "alarm_stale",
			Body:           a,
			SentAt:         now,
		})
		if err != nil {
			slog.Warn("insert stale alarm message", "pkg", "alarms", "alarm_id", a.ID, "error", err)
			continue
		}

		slog.Info("stale alarm message emitted", "pkg", "alarms", "alarm_id", a.ID, "goal", a.Goal)
	}
}
