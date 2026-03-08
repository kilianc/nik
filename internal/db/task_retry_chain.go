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

	idx := map[string]int{}
	var entries []RetryChainEntry

	for rows.Next() {
		var (
			id          string
			retryNumber int
			goal        string
			status      string
			content     string
			reportedAt  sql.NullTime
		)

		err = rows.Scan(&id, &retryNumber, &goal, &status, &content, &reportedAt)
		if err != nil {
			return nil, fmt.Errorf("scan retry chain row: %w", err)
		}

		i, exists := idx[id]
		if !exists {
			i = len(entries)
			idx[id] = i
			entries = append(entries, RetryChainEntry{
				ID:          id,
				RetryNumber: retryNumber,
				Goal:        goal,
				Status:      status,
			})
		}

		if content != "" {
			entries[i].Reports = append(entries[i].Reports, RetryChainReport{
				Content:    content,
				ReportedAt: reportedAt,
			})
		}
	}

	return entries, rows.Err()
}
