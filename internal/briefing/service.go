package briefing

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

type Service struct {
	db  *sql.DB
	cfg *config.Config
	now func() time.Time
}

func NewService(conn *sql.DB, cfg *config.Config) *Service {
	return &Service{db: conn, cfg: cfg, now: time.Now}
}

func (s *Service) HasBriefing(ctx context.Context) (bool, error) {
	return db.BriefingHasPage(ctx, s.db, s.today())
}

func (s *Service) WriteBriefing(ctx context.Context, content string) error {
	date := s.today()

	err := db.BriefingWritePage(ctx, s.db, date, content)
	if err != nil {
		return fmt.Errorf("write briefing %s: %w", date, err)
	}

	return nil
}

func (s *Service) ListTopics(ctx context.Context) ([]db.BriefingTopic, error) {
	return db.BriefingTopicList(ctx, s.db)
}

func (s *Service) AddTopic(ctx context.Context, query, reason string, contactID sql.NullString) (string, error) {
	id := db.NewID()

	err := db.BriefingTopicInsert(ctx, s.db, id, query, reason, contactID)
	if err != nil {
		return "", fmt.Errorf("add briefing topic: %w", err)
	}

	return id, nil
}

func (s *Service) RemoveTopic(ctx context.Context, id string) error {
	return db.BriefingTopicDelete(ctx, s.db, id)
}

func (s *Service) today() string {
	now := s.now()
	return now.In(s.cfg.TZ()).Format("2006-01-02")
}
