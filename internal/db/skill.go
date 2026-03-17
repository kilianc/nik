package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type SkillUpsertParams struct {
	Name        string
	Status      string
	ContentHash string
	InstallHash string
}

func SkillList(ctx context.Context, db *sql.DB) ([]Skill, error) {
	rows, err := db.QueryContext(ctx, queries.SkillList)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

	var skills []Skill
	for rows.Next() {
		var s Skill
		err = rows.Scan(
			&s.ID,
			&s.Name,
			&s.Status,
			&s.ContentHash,
			&s.InstallHash,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		skills = append(skills, s)
	}

	return skills, rows.Err()
}

func SkillUpsert(ctx context.Context, db DBTX, p SkillUpsertParams) (Skill, error) {
	var contentHash any
	if p.ContentHash != "" {
		contentHash = p.ContentHash
	}

	var installHash any
	if p.InstallHash != "" {
		installHash = p.InstallHash
	}

	row := db.QueryRowContext(ctx, queries.SkillUpsert,
		id.V7(),
		p.Name,
		p.Status,
		contentHash,
		installHash,
	)

	var s Skill
	err := row.Scan(
		&s.ID,
		&s.Name,
		&s.Status,
		&s.ContentHash,
		&s.InstallHash,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		return Skill{}, fmt.Errorf("upsert skill %s: %w", p.Name, err)
	}

	return s, nil
}
