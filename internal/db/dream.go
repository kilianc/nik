package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func DreamHasPass(ctx context.Context, db *sql.DB, date string, pass int) (bool, error) {
	var exists int

	err := db.QueryRowContext(ctx, queries.DreamCheck, date, pass).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check dream pass %s/%d: %w", date, pass, err)
	}

	return true, nil
}

func DreamStartPass(ctx context.Context, db *sql.DB, date string, pass int) error {
	_, err := db.ExecContext(ctx, queries.DreamStart, date, pass)
	if err != nil {
		return fmt.Errorf("start dream pass %s/%d: %w", date, pass, err)
	}

	return nil
}

func DreamWritePass(ctx context.Context, db *sql.DB, date string, pass int, content string) error {
	_, err := db.ExecContext(ctx, queries.DreamWrite, date, pass, content)
	if err != nil {
		return fmt.Errorf("write dream pass %s/%d: %w", date, pass, err)
	}

	return nil
}

func DreamGetPasses(ctx context.Context, db *sql.DB, date string) ([]DreamPass, error) {
	rows, err := db.QueryContext(ctx, queries.DreamPasses, date)
	if err != nil {
		return nil, fmt.Errorf("get dream passes %s: %w", date, err)
	}
	defer rows.Close()

	var out []DreamPass
	for rows.Next() {
		var p DreamPass

		err = rows.Scan(
			&p.Pass,
			&p.Content,
			&p.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dream pass: %w", err)
		}

		out = append(out, p)
	}

	return out, rows.Err()
}
