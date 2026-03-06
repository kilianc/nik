package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func TaskAllActive(ctx context.Context, db *sql.DB) ([]ActiveTask, error) {
	rows, err := db.QueryContext(ctx, queries.TaskAllActive)
	if err != nil {
		return nil, fmt.Errorf("query all active tasks: %w", err)
	}
	defer rows.Close()

	var tasks []ActiveTask
	for rows.Next() {
		t, scanErr := scanActiveTask(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan all active task: %w", scanErr)
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}
