package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func scanCrewMember(s scanner) (CrewMember, error) {
	var m CrewMember
	err := s.Scan(&m.ID, &m.Name, &m.Prompt, &m.CreatedAt)
	return m, err
}

func CrewMemberInsert(ctx context.Context, db DBTX, m CrewMember) error {
	_, err := db.ExecContext(ctx, queries.CrewMemberInsert,
		m.ID, m.Name, m.Prompt, m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert crew member %s: %w", m.Name, err)
	}

	return nil
}

func CrewMemberList(ctx context.Context, db *sql.DB) ([]CrewMember, error) {
	rows, err := db.QueryContext(ctx, queries.CrewMemberList)
	if err != nil {
		return nil, fmt.Errorf("list crew members: %w", err)
	}
	defer rows.Close()

	var members []CrewMember
	for rows.Next() {
		m, scanErr := scanCrewMember(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan crew member: %w", scanErr)
		}
		members = append(members, m)
	}

	return members, rows.Err()
}

func CrewMemberGet(ctx context.Context, db *sql.DB, idOrName string) (CrewMember, error) {
	row := db.QueryRowContext(ctx, queries.CrewMemberGet, idOrName)

	m, err := scanCrewMember(row)
	if err != nil {
		return CrewMember{}, fmt.Errorf("get crew member %s: %w", idOrName, err)
	}

	return m, nil
}
