package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

func SoulCurrent(ctx context.Context, db *sql.DB) (Soul, error) {
	var s Soul

	err := db.QueryRowContext(ctx, queries.SoulCurrent).Scan(
		&s.Version,
		&s.Content,
	)
	if err == sql.ErrNoRows {
		return Soul{}, nil
	}
	if err != nil {
		return Soul{}, fmt.Errorf("get current soul: %w", err)
	}

	return s, nil
}

func SoulInsert(ctx context.Context, db *sql.DB, content, dreamDate string) (int, error) {
	newID := id.V7()

	_, err := db.ExecContext(ctx, queries.SoulInsert, newID, content, dreamDate)
	if err != nil {
		return 0, fmt.Errorf("insert soul: %w", err)
	}

	var version int
	err = db.QueryRowContext(ctx, "SELECT version FROM soul WHERE id = ?1", newID).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read soul version: %w", err)
	}

	return version, nil
}
