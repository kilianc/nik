package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type TaskReportInsertParams struct {
	ID        string
	TaskID    string
	Kind      string
	Content   string
	CreatedAt time.Time
}

func TaskReportInsert(ctx context.Context, db *sql.DB, p TaskReportInsertParams) error {
	_, err := db.ExecContext(ctx, queries.TaskReportInsert,
		p.ID,
		p.TaskID,
		p.Kind,
		p.Content,
		p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert task report for %s: %w", p.TaskID, err)
	}

	return nil
}

func TasksNeedingAttention(ctx context.Context, db *sql.DB) ([]TaskAttention, error) {
	rows, err := db.QueryContext(ctx, queries.TaskReportUnread)
	if err != nil {
		return nil, fmt.Errorf("query tasks needing attention: %w", err)
	}
	defer rows.Close()

	var items []TaskAttention
	for rows.Next() {
		var a TaskAttention
		var metaJSON string
		var retryForTaskID sql.NullString

		err = rows.Scan(
			&a.TaskID,
			&a.Goal,
			&a.Status,
			&metaJSON,
			&retryForTaskID,
			&a.RetryNumber,
			&a.ReportIDs,
			&a.Reports,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task attention: %w", err)
		}

		a.Meta = UnmarshalMeta(metaJSON)
		a.RetryForTaskID = retryForTaskID.String
		items = append(items, a)
	}

	return items, rows.Err()
}

func TaskReportMarkRead(ctx context.Context, db *sql.DB, reportID string) error {
	_, err := db.ExecContext(ctx, queries.TaskReportMarkRead, reportID)
	if err != nil {
		return fmt.Errorf("mark report read %s: %w", reportID, err)
	}

	return nil
}
