package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type SkillEventInsertParams struct {
	Name        string
	Kind        string
	ContentHash string
	InstallHash string
}

func SkillEventInsert(ctx context.Context, db *sql.DB, p SkillEventInsertParams) (SkillEvent, error) {
	eid := id.V7()
	now := time.Now().UTC()

	var contentHash any
	if p.ContentHash != "" {
		contentHash = p.ContentHash
	}

	var installHash any
	if p.InstallHash != "" {
		installHash = p.InstallHash
	}

	_, err := db.ExecContext(ctx, queries.SkillEventInsert,
		eid,
		p.Name,
		p.Kind,
		contentHash,
		installHash,
		now,
	)
	if err != nil {
		return SkillEvent{}, err
	}

	return SkillEvent{
		ID:          eid,
		Name:        p.Name,
		Kind:        p.Kind,
		ContentHash: sql.NullString{String: p.ContentHash, Valid: p.ContentHash != ""},
		InstallHash: sql.NullString{String: p.InstallHash, Valid: p.InstallHash != ""},
		CreatedAt:   now,
	}, nil
}

func SkillEventLatestPerName(ctx context.Context, db *sql.DB) ([]SkillEvent, error) {
	rows, err := db.QueryContext(ctx, queries.SkillEventLatestPerName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SkillEvent
	for rows.Next() {
		e, scanErr := scanSkillEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		events = append(events, e)
	}

	return events, rows.Err()
}

func SkillEventList(ctx context.Context, db *sql.DB, since time.Time) ([]SkillEvent, error) {
	rows, err := db.QueryContext(ctx, queries.SkillEventList, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SkillEvent
	for rows.Next() {
		e, scanErr := scanSkillEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		events = append(events, e)
	}

	return events, rows.Err()
}

func scanSkillEvent(s scanner) (SkillEvent, error) {
	var e SkillEvent
	err := s.Scan(
		&e.ID,
		&e.Name,
		&e.Kind,
		&e.ContentHash,
		&e.InstallHash,
		&e.CreatedAt,
	)
	return e, err
}
