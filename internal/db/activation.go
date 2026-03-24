package db

import (
	"context"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type ActivationRow struct {
	ID                     string
	ConversationID         string
	TaskID                 string
	Sources                string
	Model                  string
	ReasoningEffort        string
	Verbosity              string
	InputTokens            int64
	OutputTokens           int64
	TotalTokens            int64
	CachedTokens           int64
	ReasoningTokens        int64
	MaxInputTokensPerRound int64
	MaxTotalTokensPerRound int64
	RoundCount             int
	CostUSD                float64
	ToolCallCount          int
	DurationMS             int64
	Error                  string
	Instructions           string
	Tools                  string
	ToolSchemas            string
	CreatedAt              time.Time
}

func ActivationGet(ctx context.Context, db DBTX, id string) (ActivationRow, error) {
	var r ActivationRow
	var taskID, effort, verbosity *string

	err := db.QueryRowContext(ctx, queries.ActivationGet, id).Scan(
		&r.ID,
		&r.ConversationID,
		&taskID,
		&r.Sources,
		&r.Model,
		&effort,
		&verbosity,
		&r.InputTokens,
		&r.OutputTokens,
		&r.TotalTokens,
		&r.CachedTokens,
		&r.ReasoningTokens,
		&r.MaxInputTokensPerRound,
		&r.MaxTotalTokensPerRound,
		&r.RoundCount,
		&r.CostUSD,
		&r.ToolCallCount,
		&r.DurationMS,
		&r.Error,
		&r.Instructions,
		&r.Tools,
		&r.ToolSchemas,
		&r.CreatedAt,
	)
	if err != nil {
		return r, fmt.Errorf("get activation %s: %w", id, err)
	}

	if taskID != nil {
		r.TaskID = *taskID
	}
	if effort != nil {
		r.ReasoningEffort = *effort
	}
	if verbosity != nil {
		r.Verbosity = *verbosity
	}

	return r, nil
}

func ActivationInsert(ctx context.Context, db DBTX, row ActivationRow) error {
	var effort any
	if row.ReasoningEffort != "" {
		effort = row.ReasoningEffort
	}

	var verbosity any
	if row.Verbosity != "" {
		verbosity = row.Verbosity
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
		verbosity,
		row.InputTokens,
		row.OutputTokens,
		row.TotalTokens,
		row.CachedTokens,
		row.ReasoningTokens,
		row.CostUSD,
		row.ToolCallCount,
		row.DurationMS,
		row.Error,
		row.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert activation %s: %w", row.ID, err)
	}

	return nil
}

type ActivationStatsUpdate struct {
	ReasoningEffort string
	Verbosity       string
	InputTokens     int64
	OutputTokens    int64
	TotalTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
	CostUSD         float64
	RoundCount      int
	MaxInputTokens  int64
	MaxTotalTokens  int64
	ToolCallCount   int
	DurationMS      int64
	Error           string
}

func ActivationUpdateDetail(ctx context.Context, db DBTX, id string, instructions string, tools []string, toolSchemas string) error {
	_, err := db.ExecContext(ctx, queries.ActivationUpdateDetail,
		id,
		instructions,
		MarshalStringSlice(tools),
		toolSchemas,
	)
	if err != nil {
		return fmt.Errorf("update activation detail %s: %w", id, err)
	}

	return nil
}

func ActivationUpdateStats(ctx context.Context, db DBTX, id string, s ActivationStatsUpdate) error {
	_, err := db.ExecContext(ctx, queries.ActivationUpdateStats,
		id,
		s.ReasoningEffort,
		s.Verbosity,
		s.InputTokens,
		s.OutputTokens,
		s.TotalTokens,
		s.CachedTokens,
		s.ReasoningTokens,
		s.CostUSD,
		s.RoundCount,
		s.MaxInputTokens,
		s.MaxTotalTokens,
		s.ToolCallCount,
		s.DurationMS,
		s.Error,
	)
	if err != nil {
		return fmt.Errorf("update activation stats %s: %w", id, err)
	}

	return nil
}
