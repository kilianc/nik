package db

import (
	"context"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

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
