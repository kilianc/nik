package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

func EveryToCronGet(ctx context.Context, db DBTX, naturalText string) (string, error) {
	var cronExpr string

	err := db.QueryRowContext(ctx, queries.EveryToCronGet, naturalText).Scan(&cronExpr)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("every to cron get %q: %w", naturalText, err)
	}

	return cronExpr, nil
}

func EveryToCronInsert(ctx context.Context, db DBTX, naturalText, cronExpr string) error {
	_, err := db.ExecContext(ctx, queries.EveryToCronInsert,
		naturalText,
		cronExpr,
		ISO8601MS(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("insert every to cron %q: %w", naturalText, err)
	}

	return nil
}
