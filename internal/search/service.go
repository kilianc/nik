package search

import (
	"context"
	"database/sql"

	"github.com/kciuffolo/nik/internal/db"
)

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) SearchContacts(ctx context.Context, query string, threshold float64, limit int) ([]db.ContactSearchResult, error) {
	return db.SearchContact(ctx, s.db, query, threshold, limit)
}
