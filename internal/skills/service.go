package skills

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

type Service struct {
	conn *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn}
}

func (s *Service) ListEvents(ctx context.Context, since time.Time) ([]db.SkillEvent, error) {
	return db.SkillEventList(ctx, s.conn, since)
}
