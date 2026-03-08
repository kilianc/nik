package db

import (
	"context"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type ActivationRow struct {
	ID              string
	ConversationID  string
	TaskID          string
	Sources         string
	Model           string
	ReasoningEffort string
	InputTokens     int64
	OutputTokens    int64
	TotalTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
	CostUSD         float64
	ToolCallCount   int
	DurationMS      int64
	Error           bool
	CreatedAt       time.Time
}

func ActivationInsert(ctx context.Context, db DBTX, row ActivationRow) error {
	errFlag := 0
	if row.Error {
		errFlag = 1
	}

	var effort any
	if row.ReasoningEffort != "" {
		effort = row.ReasoningEffort
	}

	var taskID any
	if row.TaskID != "" {
		taskID = row.TaskID
	}

	sources := row.Sources
	if sources == "" {
		sources = "[]"
	}

	_, err := db.ExecContext(ctx, queries.ActivationInsert,
		row.ID,
		row.ConversationID,
		taskID,
		sources,
		row.Model,
		effort,
		row.InputTokens,
		row.OutputTokens,
		row.TotalTokens,
		row.CachedTokens,
		row.ReasoningTokens,
		row.CostUSD,
		row.ToolCallCount,
		row.DurationMS,
		errFlag,
		row.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert activation %s: %w", row.ID, err)
	}

	return nil
}

type ActivationStatsUpdate struct {
	ReasoningEffort string
	InputTokens     int64
	OutputTokens    int64
	TotalTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
	CostUSD         float64
	ToolCallCount   int
	DurationMS      int64
	IsError         bool
}

func ActivationUpdateStats(ctx context.Context, db DBTX, id string, s ActivationStatsUpdate) error {
	errFlag := 0
	if s.IsError {
		errFlag = 1
	}

	_, err := db.ExecContext(ctx, queries.ActivationUpdateStats,
		id,
		s.ReasoningEffort,
		s.InputTokens,
		s.OutputTokens,
		s.TotalTokens,
		s.CachedTokens,
		s.ReasoningTokens,
		s.CostUSD,
		s.ToolCallCount,
		s.DurationMS,
		errFlag,
	)
	if err != nil {
		return fmt.Errorf("update activation stats %s: %w", id, err)
	}

	return nil
}
