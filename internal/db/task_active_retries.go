package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func TaskActiveRetries(ctx context.Context, db *sql.DB, rootID string) ([]ActiveTask, error) {
	rows, err := db.QueryContext(ctx, queries.TaskActiveRetries, rootID)
	if err != nil {
		return nil, fmt.Errorf("query active retries for %s: %w", rootID, err)
	}
	defer rows.Close()

	var tasks []ActiveTask
	for rows.Next() {
		t, scanErr := scanActiveTask(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan active retry: %w", scanErr)
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}
