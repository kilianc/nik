package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func SettingGet(ctx context.Context, db DBTX, key string) (*Setting, error) {
	row := db.QueryRowContext(ctx, queries.SettingGet, key)

	var s Setting
	err := row.Scan(
		&s.Key,
		&s.Value,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get setting %s: %w", key, err)
	}

	return &s, nil
}

func SettingSet(ctx context.Context, db DBTX, key, value string) error {
	_, err := db.ExecContext(ctx, queries.SettingSet, key, value)
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}

	return nil
}
