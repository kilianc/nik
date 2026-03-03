package stats

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

type Recorder struct {
	conn *sql.DB
}

func NewRecorder(conn *sql.DB) *Recorder {
	return &Recorder{conn: conn}
}

func (r *Recorder) Record(ctx context.Context, stats brain.ActivationStats) {
	actID := id.V7()

	err := db.ActivationInsert(ctx, r.conn, db.ActivationRow{
		ID:              actID,
		Source:          stats.Meta["source"],
		SourceID:        stats.Meta["source_id"],
		Model:           stats.Model,
		ReasoningEffort: stats.ReasoningEffort,
		InputTokens:     stats.Usage.InputTokens,
		OutputTokens:    stats.Usage.OutputTokens,
		TotalTokens:     stats.Usage.TotalTokens,
		CachedTokens:    stats.Usage.CachedTokens,
		ReasoningTokens: stats.Usage.ReasoningTokens,
		CostUSD:         stats.CostUSD,
		ToolCallCount:   len(stats.ToolCalls),
		DurationMS:      stats.DurationMS,
		Error:           stats.Error,
		CreatedAt:       time.Now().UTC(),
	})
	if err != nil {
		slog.Error("record activation stats", "pkg", "stats", "error", err)
		return
	}

	tcRows := make([]db.ToolCallRow, len(stats.ToolCalls))
	for i, tc := range stats.ToolCalls {
		tcRows[i] = db.ToolCallRow{
			Name:       tc.Name,
			DurationMS: tc.DurationMS,
			Error:      tc.Error,
		}
	}

	err = db.ToolCallInsert(ctx, r.conn, actID, tcRows)
	if err != nil {
		slog.Error("record tool call stats", "pkg", "stats", "error", err)
	}
}
