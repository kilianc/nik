package journal

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

func (s *Service) HasPage(ctx context.Context) (bool, error) {
	return db.JournalHasPage(ctx, s.db, s.today())
}

func (s *Service) Start(ctx context.Context) error {
	return db.JournalStartPage(ctx, s.db, s.today())
}

func (s *Service) WritePage(ctx context.Context, content string) error {
	date := s.today()

	err := db.JournalWritePage(ctx, s.db, date, content)
	if err != nil {
		return fmt.Errorf("write page %s: %w", date, err)
	}

	return nil
}

// today returns the date the journal reflects on. at midnight, this is
// the day that just ended, not the one starting.
func (s *Service) today() string {
	journalAt := s.cfg.JournalAt(s.now())
	effectiveDay := journalAt.Add(-time.Minute)

	return effectiveDay.In(s.cfg.TZ()).Format("2006-01-02")
}
