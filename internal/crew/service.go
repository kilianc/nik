package crew

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

type Service struct {
	conn *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn}
}

func (s *Service) Hire(ctx context.Context, name, prompt string) (db.CrewMember, error) {
	m := db.CrewMember{
		ID:        id.V7(),
		Name:      name,
		Prompt:    prompt,
		CreatedAt: time.Now().UTC(),
	}

	err := db.CrewMemberInsert(ctx, s.conn, m)
	if err != nil {
		return db.CrewMember{}, err
	}

	return m, nil
}

func (s *Service) Roster(ctx context.Context) (string, error) {
	members, err := db.CrewMemberList(ctx, s.conn)
	if err != nil {
		return "", err
	}

	if len(members) == 0 {
		return "You haven't hired anyone yet. When you need to delegate, hire your first crew member -- it's a big moment.", nil
	}

	var b strings.Builder
	for _, m := range members {
		prompt := m.Prompt
		if len(prompt) > 80 {
			prompt = prompt[:80] + "..."
		}
		fmt.Fprintf(&b, "- **%s**: %s\n", m.Name, prompt)
	}

	return b.String(), nil
}

func (s *Service) Get(ctx context.Context, idOrName string) (db.CrewMember, error) {
	return db.CrewMemberGet(ctx, s.conn, idOrName)
}
