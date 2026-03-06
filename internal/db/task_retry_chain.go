package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func TaskRetryChain(ctx context.Context, db *sql.DB, rootID string) ([]RetryChainEntry, error) {
	rows, err := db.QueryContext(ctx, queries.TaskRetryChain, rootID)
	if err != nil {
		return nil, fmt.Errorf("query retry chain for %s: %w", rootID, err)
	}
	defer rows.Close()

	var entries []RetryChainEntry
	for rows.Next() {
		var e RetryChainEntry

		err = rows.Scan(
			&e.ID,
			&e.RetryNumber,
			&e.Goal,
			&e.Status,
			&e.Reports,
		)
		if err != nil {
			return nil, fmt.Errorf("scan retry chain entry: %w", err)
		}

		entries = append(entries, e)
	}

	return entries, rows.Err()
}
