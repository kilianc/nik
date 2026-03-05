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

func TaskReportUnread(ctx context.Context, db *sql.DB) ([]TaskReport, error) {
	rows, err := db.QueryContext(ctx, queries.TaskReportUnread)
	if err != nil {
		return nil, fmt.Errorf("query unread reports: %w", err)
	}
	defer rows.Close()

	var reports []TaskReport
	for rows.Next() {
		var r TaskReport
		var metaJSON string

		err = rows.Scan(
			&r.ID,
			&r.TaskID,
			&r.Kind,
			&r.Content,
			&r.ReportedAt,
			&r.CreatedAt,
			&metaJSON,
			&r.Goal,
			&r.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}

		r.Meta = UnmarshalMeta(metaJSON)
		reports = append(reports, r)
	}

	return reports, rows.Err()
}

func TaskReportMarkRead(ctx context.Context, db *sql.DB, reportID string) error {
	_, err := db.ExecContext(ctx, queries.TaskReportMarkRead, reportID)
	if err != nil {
		return fmt.Errorf("mark report read %s: %w", reportID, err)
	}

	return nil
}
