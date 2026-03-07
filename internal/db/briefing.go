package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

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
