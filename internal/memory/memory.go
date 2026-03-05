package memory

import (
	"database/sql"

	"github.com/kciuffolo/nik/internal/llm"
)

type Service struct {
	conn *sql.DB
	llm  *llm.Client
}

func NewService(conn *sql.DB, llmClient *llm.Client) *Service {
	return &Service{conn: conn, llm: llmClient}
}
