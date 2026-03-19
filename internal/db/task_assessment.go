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
	TaskID                  string
	ActivationID            string
	EffectivenessScore      int
	EffectivenessFeedback   string
	ExpectedDurationSeconds int
	DurationFeedback        string
	ToolFeedback            string
	SkillFeedback           string
	Recommendations         string
}

func TaskAssessmentInsert(ctx context.Context, db *sql.DB, p TaskAssessmentInsertParams) error {
	_, err := db.ExecContext(ctx, queries.TaskAssessmentInsert,
		id.V7(),
		p.TaskID,
		p.ActivationID,
		p.EffectivenessScore,
		p.EffectivenessFeedback,
		p.ExpectedDurationSeconds,
		p.DurationFeedback,
		p.ToolFeedback,
		p.SkillFeedback,
		p.Recommendations,
	)
	if err != nil {
		return fmt.Errorf("insert task assessment for %s: %w", p.TaskID, err)
	}

	return nil
}

func TaskAssessmentToolCallList(ctx context.Context, db *sql.DB, activationID string) ([]ToolCallInfo, error) {
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
			&tc.Round,
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

func TaskReportList(ctx context.Context, db *sql.DB, taskID string) ([]TaskReportRow, error) {
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
