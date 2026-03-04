package dream

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

const totalPasses = 5

type Service struct {
	db  *sql.DB
	cfg *config.Config
	now func() time.Time
}

func NewService(conn *sql.DB, cfg *config.Config) *Service {
	return &Service{db: conn, cfg: cfg, now: time.Now}
}

func (s *Service) HasPass(ctx context.Context, pass int) (bool, error) {
	return db.DreamHasPass(ctx, s.db, s.tonight(), pass)
}

func (s *Service) StartPass(ctx context.Context, pass int) error {
	return db.DreamStartPass(ctx, s.db, s.tonight(), pass)
}

func (s *Service) WriteDream(ctx context.Context, pass int, content string) error {
	date := s.tonight()

	err := db.DreamWritePass(ctx, s.db, date, pass, content)
	if err != nil {
		return fmt.Errorf("write dream %s pass %d: %w", date, pass, err)
	}

	return nil
}

func (s *Service) GetPasses(ctx context.Context) ([]db.DreamPass, error) {
	return db.DreamGetPasses(ctx, s.db, s.tonight())
}

func (s *Service) CurrentSoul(ctx context.Context) (string, error) {
	soul, err := db.SoulCurrent(ctx, s.db)
	if err != nil {
		return "", fmt.Errorf("read current soul: %w", err)
	}

	return soul.Content, nil
}

func (s *Service) WriteSoul(ctx context.Context, content string) (int, error) {
	date := s.tonight()

	version, err := db.SoulInsert(ctx, s.db, content, date)
	if err != nil {
		return 0, fmt.Errorf("write soul for %s: %w", date, err)
	}

	return version, nil
}

// tonight returns the date the dream cycle belongs to. dreams that run
// after midnight belong to the previous calendar day.
func (s *Service) tonight() string {
	now := s.now()
	dreamStart := s.cfg.DreamAt(now, 1)
	prev := dreamStart.AddDate(0, 0, -1)

	return prev.In(s.cfg.TZ()).Format("2006-01-02")
}
