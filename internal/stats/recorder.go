package stats

import (
	"context"
	"database/sql"
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

func (r *Recorder) OnStart(ctx context.Context, model string) {
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

func (r *Recorder) OnToolCall(ctx context.Context, name string, round int, args string, result string, duration time.Duration, isError bool) {
	meta := metaFromCtx(ctx)
	actID := meta["activation_id"]
	if actID == "" {
		return
	}

	err := db.ToolCallInsertOne(ctx, r.conn, db.ToolCallInsertParams{
		ActivationID: actID,
		Name:         name,
		Round:        round,
		Input:        args,
		Output:       result,
		Duration:     duration,
		IsError:      isError,
	})
	if err != nil {
		slog.Warn("record tool call", "pkg", "stats", "activation_id", actID, "error", err)
	}
}

func (r *Recorder) OnFinish(ctx context.Context, model, reasoningEffort string, usage llm.Usage, rounds llm.RoundStats, toolCalls int, durationMS int64, output string, processErr error) {
	meta := metaFromCtx(ctx)
	actID := meta["activation_id"]
	if actID == "" {
		return
	}

	var errText string
	if processErr != nil {
		errText = processErr.Error()
	}

	// detach so the write completes even if the activation context is canceled
	ctx = context.WithoutCancel(ctx)

	err := db.ActivationUpdateStats(ctx, r.conn, actID, db.ActivationStatsUpdate{
		ReasoningEffort: reasoningEffort,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		TotalTokens:     usage.TotalTokens,
		CachedTokens:    usage.CachedTokens,
		ReasoningTokens: usage.ReasoningTokens,
		CostUSD:         llm.ComputeCost(model, usage.InputTokens, usage.OutputTokens, usage.CachedTokens),
		RoundCount:      rounds.RoundCount,
		MaxInputTokens:  rounds.MaxInputTokensPerRound,
		MaxTotalTokens:  rounds.MaxTotalTokensPerRound,
		ToolCallCount:   toolCalls,
		DurationMS:      durationMS,
		Error:           errText,
		Output:          output,
	})
	if err != nil {
		slog.Warn("update activation stats", "pkg", "stats", "activation_id", actID, "error", err)
	}
}

func (r *Recorder) OnDetail(ctx context.Context, instructions string, userInput string, tools []string, reasoningSummaries []string) {
	meta := metaFromCtx(ctx)
	actID := meta["activation_id"]
	if actID == "" {
		return
	}

	ctx = context.WithoutCancel(ctx)

	err := db.ActivationDetailInsert(ctx, r.conn, db.ActivationDetailParams{
		ActivationID:       actID,
		Instructions:       instructions,
		UserInput:          userInput,
		Tools:              tools,
		ReasoningSummaries: reasoningSummaries,
	})
	if err != nil {
		slog.Warn("insert activation detail", "pkg", "stats", "activation_id", actID, "error", err)
	}
}

func metaFromCtx(ctx context.Context) map[string]string {
	meta, _ := ctx.Value("meta").(map[string]string)
	if meta == nil {
		return map[string]string{}
	}
	return meta
}

// compile-time check
var _ llm.CompletionObserver = (*Recorder)(nil)
