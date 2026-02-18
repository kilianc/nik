package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func BriefingHasPage(ctx context.Context, db *sql.DB, date string) (bool, error) {
	var exists int

	err := db.QueryRowContext(ctx, queries.BriefingCheck, date).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check briefing page %s: %w", date, err)
	}

	return true, nil
}

func BriefingGetPage(ctx context.Context, db *sql.DB, date string) (string, error) {
	var content string

	err := db.QueryRowContext(ctx, queries.BriefingGet, date).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get briefing page %s: %w", date, err)
	}

	return content, nil
}

func BriefingWritePage(ctx context.Context, db *sql.DB, date, content string) error {
	_, err := db.ExecContext(ctx, queries.BriefingWrite, date, content)
	if err != nil {
		return fmt.Errorf("write briefing page %s: %w", date, err)
	}

	return nil
}

type BriefingTopic struct {
	ID        string
	Query     string
	Reason    string
	ContactID sql.NullString
	CreatedAt time.Time
}

func BriefingTopicList(ctx context.Context, db *sql.DB) ([]BriefingTopic, error) {
	rows, err := db.QueryContext(ctx, queries.BriefingTopicList)
	if err != nil {
		return nil, fmt.Errorf("list briefing topics: %w", err)
	}
	defer rows.Close()

	var out []BriefingTopic
	for rows.Next() {
		var t BriefingTopic

		err = rows.Scan(
			&t.ID,
			&t.Query,
			&t.Reason,
			&t.ContactID,
			&t.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan briefing topic: %w", err)
		}

		out = append(out, t)
	}

	return out, rows.Err()
}

func BriefingTopicInsert(ctx context.Context, db *sql.DB, id, query, reason string, contactID sql.NullString) error {
	_, err := db.ExecContext(ctx, queries.BriefingTopicInsert, id, query, reason, contactID)
	if err != nil {
		return fmt.Errorf("insert briefing topic: %w", err)
	}

	return nil
}

func BriefingTopicDelete(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, queries.BriefingTopicDelete, id)
	if err != nil {
		return fmt.Errorf("delete briefing topic %s: %w", id, err)
	}

	return nil
}
