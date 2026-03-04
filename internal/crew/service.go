package crew

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type Member struct {
	ID        string
	Name      string
	Prompt    string
	CreatedAt time.Time
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Hire(ctx context.Context, name, prompt string) (Member, error) {
	m := Member{
		ID:        id.V7(),
		Name:      name,
		Prompt:    prompt,
		CreatedAt: time.Now().UTC(),
	}

	_, err := s.db.ExecContext(ctx, queries.CrewMemberInsert,
		m.ID,
		m.Name,
		m.Prompt,
		m.CreatedAt,
	)
	if err != nil {
		return Member{}, fmt.Errorf("insert crew member %s: %w", m.Name, err)
	}

	return m, nil
}

func (s *Service) List(ctx context.Context) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, queries.CrewMemberList)
	if err != nil {
		return nil, fmt.Errorf("list crew members: %w", err)
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var m Member

		err = rows.Scan(
			&m.ID,
			&m.Name,
			&m.Prompt,
			&m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan crew member: %w", err)
		}

		members = append(members, m)
	}

	return members, rows.Err()
}

func (s *Service) Roster(ctx context.Context) (string, error) {
	members, err := s.List(ctx)
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

func (s *Service) Get(ctx context.Context, idOrName string) (Member, error) {
	row := s.db.QueryRowContext(ctx, queries.CrewMemberGet, idOrName)

	var m Member

	err := row.Scan(
		&m.ID,
		&m.Name,
		&m.Prompt,
		&m.CreatedAt,
	)
	if err != nil {
		return Member{}, fmt.Errorf("get crew member %s: %w", idOrName, err)
	}

	return m, nil
}
