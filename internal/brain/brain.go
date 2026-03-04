package brain

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
)

// ActivationStats holds non-sensitive metadata for a single brain activation.
type ActivationStats struct {
	Meta            map[string]string
	Model           string
	ReasoningEffort string
	Usage           llm.Usage
	CostUSD         float64
	ToolCalls       []ToolCallStats
	DurationMS      int64
	Error           bool
}

type ToolCallStats struct {
	Name       string
	DurationMS int64
	Error      bool
}

type StatsRecorder func(ctx context.Context, stats ActivationStats)

type Brain struct {
	cfg           *config.Config
	llm           *llm.Client
	toolDefs      []llm.ToolDef
	toolExec      map[string]llm.ToolExecutor
	privileged    map[string]bool
	dataSources   []DataSource
	soulReader    func(ctx context.Context) (string, error)
	statsRecorder StatsRecorder
	now           func() time.Time

	claimed     *SyncSet
	activations *SyncSet
}

// IsActive reports whether an activation with the given run ID is still alive.
func (b *Brain) IsActive(activationID string) bool {
	return b.activations.Has(activationID)
}

func New(cfg *config.Config, llmClient *llm.Client) *Brain {
	return &Brain{
		cfg:         cfg,
		llm:         llmClient,
		toolExec:    make(map[string]llm.ToolExecutor),
		privileged:  make(map[string]bool),
		now:         time.Now,
		claimed:     NewSyncSet(),
		activations: NewSyncSet(),
	}
}

func (b *Brain) SetSoulReader(fn func(ctx context.Context) (string, error)) {
	b.soulReader = fn
}

func (b *Brain) SetStatsRecorder(fn StatsRecorder) {
	b.statsRecorder = fn
}

const activationTimeout = 20 * time.Minute

// Awake starts the main loop. Brain wakes up on each tick, perceives
// stimuli, and activates on anything new. Blocks until ctx is cancelled.
func (b *Brain) Awake(ctx context.Context, pollInterval time.Duration) {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}

	slog.Info("brain awake", "pkg", "brain", "data_sources", len(b.dataSources), "poll", pollInterval)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("brain sleeping", "pkg", "brain")
			return
		case <-ticker.C:
			b.perceive(ctx)
		}
	}
}

func (b *Brain) perceive(ctx context.Context) {
	var outputs []DataSourceOutput

	for _, source := range b.dataSources {
		output, err := source.Check(ctx)
		if err != nil {
			slog.Error("check data source failed", "pkg", "brain", "error", err)
			continue
		}
		outputs = append(outputs, output...)
	}

	for _, output := range outputs {
		sourceID := output.Meta["source_id"]
		if sourceID != "" {
			key := output.Meta["source"] + ":" + sourceID
			if !b.claimed.TrySet(key) {
				continue
			}
		}

		go b.activate(ctx, output)
	}
}

func (b *Brain) activate(ctx context.Context, output DataSourceOutput) {
	if output.Meta == nil {
		output.Meta = make(map[string]string)
	}

	sourceID := output.Meta["source_id"]
	if sourceID != "" {
		key := output.Meta["source"] + ":" + sourceID
		defer b.claimed.Delete(key)
	}

	activationID := id.Short(8)

	b.activations.Set(activationID)
	defer b.activations.Delete(activationID)

	output.Meta["activation_id"] = activationID

	ctx = context.WithValue(ctx, "meta", output.Meta)

	if output.Processing != nil {
		err := output.Processing(ctx)
		if err != nil {
			slog.Error("processing callback failed", "pkg", "brain", "error", err)
		}
	}

	_, _, err := b.think(ctx, output.Lines)
	if err != nil {
		// think errors are logged but never block Processed -- datasources
		// must always run their cleanup (mark-read, release locks, etc.)
		slog.Error("think failed", "pkg", "brain", "error", err)
	}

	if output.Processed != nil {
		err = output.Processed(ctx)
		if err != nil {
			slog.Error("processed callback failed", "pkg", "brain", "error", err)
		}
	}
}

const maxThinkAttempts = 2

func (b *Brain) think(ctx context.Context, input []string) (string, llm.Usage, error) {
	start := time.Now()

	now := b.now
	if now == nil {
		now = time.Now
	}

	userInput := strings.Join(input, "\n")

	thinkCtx, cancel := context.WithTimeout(ctx, activationTimeout)
	defer cancel()

	meta, _ := ctx.Value("meta").(map[string]string)
	tools := b.toolsForContext(ctx)
	executor := b.toolExecutor()

	var totalUsage llm.Usage
	var allToolCalls []llm.ToolCallRecord
	var reasoningEffort string
	var thinkErr error

	defer func() {
		if b.statsRecorder == nil {
			return
		}

		tcStats := make([]ToolCallStats, len(allToolCalls))
		for i, tc := range allToolCalls {
			tcStats[i] = ToolCallStats{
				Name:       tc.Name,
				DurationMS: tc.DurationMS,
				Error:      tc.Error,
			}
		}

		b.statsRecorder(ctx, ActivationStats{
			Meta:            meta,
			Model:           b.llm.Model(),
			ReasoningEffort: reasoningEffort,
			Usage:           totalUsage,
			CostUSD:         llm.ComputeCost(b.llm.Model(), totalUsage.InputTokens, totalUsage.OutputTokens, totalUsage.CachedTokens),
			ToolCalls:       tcStats,
			DurationMS:      time.Since(start).Milliseconds(),
			Error:           thinkErr != nil,
		})
	}()

	for attempt := range maxThinkAttempts {
		retry := attempt > 0
		if retry {
			slog.Warn("no tool calls produced, retrying", "pkg", "brain", "attempt", attempt)
		}

		instructions, err := b.loadInstructions(now(), retry)
		if err != nil {
			thinkErr = err
			return "", totalUsage, err
		}

		output, usage, toolCalls, extra, processErr := b.llm.Complete(thinkCtx, instructions, userInput, tools, executor)
		b.writeDebugRecord(meta, instructions, userInput, output, tools, toolCalls, extra, usage, processErr)

		allToolCalls = append(allToolCalls, toolCalls...)
		if extra.ReasoningEffort != "" {
			reasoningEffort = extra.ReasoningEffort
		}

		totalUsage.InputTokens += usage.InputTokens
		totalUsage.OutputTokens += usage.OutputTokens
		totalUsage.TotalTokens += usage.TotalTokens
		totalUsage.CachedTokens += usage.CachedTokens
		totalUsage.ReasoningTokens += usage.ReasoningTokens

		if processErr != nil {
			thinkErr = processErr
			return "", totalUsage, processErr
		}

		if len(toolCalls) > 0 {
			return output, totalUsage, nil
		}
	}

	thinkErr = errors.New("no tool calls produced after retries")
	return "", totalUsage, thinkErr
}
