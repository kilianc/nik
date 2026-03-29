package stats

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

type Recorder struct {
	conn *sql.DB
}

func NewRecorder(conn *sql.DB) *Recorder {
	return &Recorder{conn: conn}
}

func (r *Recorder) Start(ctx context.Context, model string) {
	meta := metaFromCtx(ctx)
	actID := meta["activation_id"]
	if actID == "" {
		return
	}

	err := db.ActivationInsert(ctx, r.conn, db.ActivationRow{
		ID:             actID,
		ConversationID: meta["conversation_id"],
		TaskID:         meta["task_id"],
		Sources:        meta["sources"],
		Model:          model,
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		slog.Warn("create activation", "pkg", "stats", "activation_id", actID, "error", err)
	}
}

func (r *Recorder) Round(ctx context.Context, round, attempt int, messages string, reasoningSummaries []string, usage llm.Usage) string {
	meta := metaFromCtx(ctx)
	actID := meta["activation_id"]
	if actID == "" {
		return ""
	}

	roundID, err := db.ActivationRoundInsert(ctx, r.conn, db.ActivationRoundInsertParams{
		ActivationID:       actID,
		Round:              round,
		Messages:           messages,
		ReasoningSummaries: reasoningSummaries,
		InputTokens:        usage.InputTokens,
		OutputTokens:       usage.OutputTokens,
		CachedTokens:       usage.CachedTokens,
		ReasoningTokens:    usage.ReasoningTokens,
	})
	if err != nil {
		slog.Warn("record activation round", "pkg", "stats", "activation_id", actID, "round", round, "error", err)
		return ""
	}

	return roundID
}

func (r *Recorder) ToolCall(ctx context.Context, roundID string, call llm.ToolCall, result llm.ExecResult) {
	meta := metaFromCtx(ctx)
	actID := meta["activation_id"]
	if actID == "" {
		return
	}

	err := db.ToolCallInsert(ctx, r.conn, db.ToolCallInsertParams{
		ActivationID:      actID,
		ActivationRoundID: roundID,
		Name:              call.Name,
		Input:             call.Arguments,
		Output:            result.Output,
		Duration:          result.Elapsed,
		IsError:           result.IsErr,
	})
	if err != nil {
		var sizeErr db.ToolCallTooLargeError
		if errors.As(err, &sizeErr) {
			slog.Warn("tool call payload exceeds db guardrail",
				"pkg", "stats",
				"activation_id", actID,
				"round_id", roundID,
				"tool", call.Name,
				"field", sizeErr.Field,
				"bytes", sizeErr.Bytes,
				"max_bytes", sizeErr.MaxBytes,
				"error", err,
			)
			return
		}

		slog.Warn("record tool call", "pkg", "stats", "activation_id", actID, "error", err)
	}
}

func (r *Recorder) Sync(ctx context.Context, stats llm.ActivationStats) {
	meta := metaFromCtx(ctx)
	actID := meta["activation_id"]
	if actID == "" {
		return
	}

	err := db.ActivationUpdate(ctx, r.conn, actID, db.ActivationUpdateParams{
		ReasoningEffort: stats.ReasoningEffort,
		Verbosity:       stats.Verbosity,
		InputTokens:     stats.Usage.InputTokens,
		OutputTokens:    stats.Usage.OutputTokens,
		TotalTokens:     stats.Usage.TotalTokens,
		CachedTokens:    stats.Usage.CachedTokens,
		ReasoningTokens: stats.Usage.ReasoningTokens,
		CostUSD:         llm.ComputeCost(stats.Model, stats.Usage.InputTokens, stats.Usage.OutputTokens, stats.Usage.CachedTokens),
		RoundCount:      stats.Rounds.RoundCount,
		MaxInputTokens:  stats.Rounds.MaxInputTokensPerRound,
		MaxTotalTokens:  stats.Rounds.MaxTotalTokensPerRound,
		ToolCallCount:   stats.ToolCallCount,
		DurationMS:      stats.DurationMS,
	})
	if err != nil {
		slog.Warn("sync activation", "pkg", "stats", "activation_id", actID, "error", err)
	}
}

func (r *Recorder) Finish(ctx context.Context, stats llm.ActivationStats) {
	meta := metaFromCtx(ctx)
	actID := meta["activation_id"]
	if actID == "" {
		return
	}

	ctx = context.WithoutCancel(ctx)

	schemas := "[]"
	if len(stats.ToolSchemas) > 0 {
		data, jsonErr := json.Marshal(stats.ToolSchemas)
		if jsonErr != nil {
			slog.Warn("marshal tool schemas", "pkg", "stats", "activation_id", actID, "error", jsonErr)
		} else {
			schemas = string(data)
		}
	}

	instructions := stats.Instructions
	err := db.ActivationUpdate(ctx, r.conn, actID, db.ActivationUpdateParams{
		Instructions:    &instructions,
		Tools:           stats.Tools,
		ToolSchemas:     &schemas,
		ReasoningEffort: stats.ReasoningEffort,
		Verbosity:       stats.Verbosity,
		InputTokens:     stats.Usage.InputTokens,
		OutputTokens:    stats.Usage.OutputTokens,
		TotalTokens:     stats.Usage.TotalTokens,
		CachedTokens:    stats.Usage.CachedTokens,
		ReasoningTokens: stats.Usage.ReasoningTokens,
		CostUSD:         llm.ComputeCost(stats.Model, stats.Usage.InputTokens, stats.Usage.OutputTokens, stats.Usage.CachedTokens),
		RoundCount:      stats.Rounds.RoundCount,
		MaxInputTokens:  stats.Rounds.MaxInputTokensPerRound,
		MaxTotalTokens:  stats.Rounds.MaxTotalTokensPerRound,
		ToolCallCount:   stats.ToolCallCount,
		DurationMS:      stats.DurationMS,
		Error:           stats.Error,
	})
	if err != nil {
		slog.Warn("update activation", "pkg", "stats", "activation_id", actID, "error", err)
	}
}

func metaFromCtx(ctx context.Context) map[string]string {
	meta, _ := ctx.Value("meta").(map[string]string)
	if meta == nil {
		return map[string]string{}
	}
	return meta
}

var _ llm.ActivationRecorder = (*Recorder)(nil)
