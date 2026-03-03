package db

import (
	"context"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type ActivationRow struct {
	ID              string
	Source          string
	SourceID        string
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

	_, err := db.ExecContext(ctx, queries.ActivationInsert,
		row.ID,
		row.Source,
		row.SourceID,
		row.Model,
		row.ReasoningEffort,
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
