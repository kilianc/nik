package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type TaskAssessmentInsertParams struct {
	TaskID        string
	ActivationID  string
	Effectiveness int
	ToolFeedback  string
	SkillFeedback string
	Suggestions   string
}

func TaskAssessmentInsert(ctx context.Context, db *sql.DB, p TaskAssessmentInsertParams) error {
	_, err := db.ExecContext(ctx, queries.TaskAssessmentInsert,
		id.V7(),
		p.TaskID,
		p.ActivationID,
		p.Effectiveness,
		p.ToolFeedback,
		p.SkillFeedback,
		p.Suggestions,
	)
	if err != nil {
		return fmt.Errorf("insert task assessment for %s: %w", p.TaskID, err)
	}

	return nil
}

func TaskAllToolCalls(ctx context.Context, db *sql.DB, activationID string) ([]ToolCallInfo, error) {
	rows, err := db.QueryContext(ctx, queries.TaskAssessmentToolCalls, activationID)
	if err != nil {
		return nil, fmt.Errorf("query tool calls for activation %s: %w", activationID, err)
	}
	defer rows.Close()

	var calls []ToolCallInfo
	for rows.Next() {
		var tc ToolCallInfo
		var errFlag int

		err = rows.Scan(
			&tc.Name,
			&tc.Input,
			&tc.Output,
			&tc.DurationMS,
			&errFlag,
			&tc.At,
		)
		if err != nil {
			return nil, fmt.Errorf("scan tool call: %w", err)
		}

		tc.Error = errFlag != 0
		calls = append(calls, tc)
	}

	return calls, rows.Err()
}

type TaskReportRow struct {
	ID        string
	Status    string
	Content   string
	CreatedAt time.Time
}

func TaskReportsByTask(ctx context.Context, db *sql.DB, taskID string) ([]TaskReportRow, error) {
	rows, err := db.QueryContext(ctx, queries.TaskReportByTask, taskID)
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
