package db

import (
	"context"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

type ActivationStatsUpdate struct {
	InputTokens     int64
	OutputTokens    int64
	TotalTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
	CostUSD         float64
	ToolCallCount   int
	DurationMS      int64
}

func ActivationUpdateStats(ctx context.Context, db DBTX, id string, s ActivationStatsUpdate) error {
	_, err := db.ExecContext(ctx, queries.ActivationUpdateStats,
		id,
		s.InputTokens,
		s.OutputTokens,
		s.TotalTokens,
		s.CachedTokens,
		s.ReasoningTokens,
		s.CostUSD,
		s.ToolCallCount,
		s.DurationMS,
	)
	if err != nil {
		return fmt.Errorf("update activation stats %s: %w", id, err)
	}

	return nil
}
