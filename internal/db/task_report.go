package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type TaskReportRow struct {
	ID        string
	Status    string
	Content   string
	CreatedAt time.Time
}

func TaskReportList(ctx context.Context, db *sql.DB, taskID string) ([]TaskReportRow, error) {
	rows, err := db.QueryContext(ctx, queries.TaskReportList, taskID)
	if err != nil {
		return nil, fmt.Errorf("query reports for task %s: %w", taskID, err)
	}
	defer rows.Close()

	var reports []TaskReportRow
	for rows.Next() {
		var r TaskReportRow

		err = rows.Scan(
			&r.ID,
			&r.Status,
			&r.Content,
			&r.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task report: %w", err)
		}

		reports = append(reports, r)
	}

	return reports, rows.Err()
}
