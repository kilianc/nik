package memory

import (
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/llm"
)

type Memory struct {
	ID        string
	Content   string
	Metadata  map[string]any
	Source    string
	SourceID string
	CreatedAt time.Time
	Score     float64
}

type Service struct {
	db  *sql.DB
	llm *llm.Client
}

func NewService(conn *sql.DB, llmClient *llm.Client) *Service {
	return &Service{db: conn, llm: llmClient}
}
