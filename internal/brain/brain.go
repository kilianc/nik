package brain

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

type Brain struct {
	cfg             *config.Config
	llm             llm.Completer
	toolDefs        []llm.ToolDef
	toolExec        map[string]llm.ToolExecutor
	privileged      map[string]bool
	sensor          Sensor
	reflexes        []Reflex
	recaller        func(ctx context.Context, stimulus string) string
	workerToolNames []string
	now             func() time.Time

	claimed *SyncSet
	wg      sync.WaitGroup
}

func New(cfg *config.Config, llmClient llm.Completer) *Brain {
	return &Brain{
		cfg:        cfg,
		llm:        llmClient,
		toolExec:   make(map[string]llm.ToolExecutor),
		privileged: make(map[string]bool),
		now:        time.Now,
		claimed:    NewSyncSet(),
	}
}

func (b *Brain) SetWorkerToolNames(names []string) {
	b.workerToolNames = names
}

func (b *Brain) SetRecaller(fn func(ctx context.Context, stimulus string) string) {
	b.recaller = fn
}

const activationTimeout = 20 * time.Minute

// Awake starts the main loop. Brain wakes up on each tick, perceives
// stimuli, and activates on anything new. Blocks until ctx is cancelled.
func (b *Brain) Awake(ctx context.Context, pollInterval time.Duration) {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}

	slog.Info("brain awake", "pkg", "brain", "poll", pollInterval)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("brain draining activations", "pkg", "brain")
			b.wg.Wait()
			slog.Info("brain sleeping", "pkg", "brain")
			return
		case <-ticker.C:
			b.perceive(ctx)
		}
	}
}

// perceive runs reflexes then polls the sensor for new stimuli; each stimulus is handed to activate.
func (b *Brain) perceive(ctx context.Context) {
	_, err := b.cfg.ReloadIfChanged()
	if err != nil {
		slog.Warn("config reload failed", "pkg", "brain", "error", err)
	}

	for _, r := range b.reflexes {
		r(ctx)
	}

	if b.sensor == nil {
		return
	}

	stimuli, err := b.sensor.Check(ctx)
	if err != nil {
		slog.Error("sensor check failed", "pkg", "brain", "error", err)
		return
	}

	for _, s := range stimuli {
		convID := s.Meta["conversation_id"]
		if convID == "" {
			panic("stimulus with empty conversation_id")
		}

		if !b.claimed.TrySet("conversation:" + convID) {
			continue
		}

		b.wg.Add(1)
		go b.activate(context.WithoutCancel(ctx), s)
	}
}

// activate consumes a single stimulus from perceive and runs the think loop against it.
func (b *Brain) activate(ctx context.Context, output Stimulus) {
	defer b.wg.Done()
	start := time.Now()

	if output.Meta == nil {
		output.Meta = make(map[string]string)
	}

	convID := output.Meta["conversation_id"]
	defer b.claimed.Delete("conversation:" + convID)

	sources := output.Meta["sources"]
	slog.Info("activation starting", "pkg", "brain", "conversation_id", convID, "sources", sources)

	ctx = context.WithValue(ctx, "meta", output.Meta)

	_, usage, err := b.think(ctx, func() string {
		return b.sensor.Get(ctx, convID)
	})
	elapsed := time.Since(start)

	if err != nil {
		slog.Error("activation failed", "pkg", "brain", "conversation_id", convID, "sources", sources, "elapsed", elapsed, "error", err)
		return
	}

	slog.Info("activation completed", "pkg", "brain",
		"conversation_id", convID,
		"sources", sources,
		"elapsed", elapsed,
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
		"reasoning_tokens", usage.ReasoningTokens,
	)
}

const maxThinkAttempts = 2

func (b *Brain) think(ctx context.Context, getInput func() string) (string, llm.Usage, error) {
	userInput := getInput()

	var recall string
	if b.recaller != nil {
		recall = b.recaller(ctx, userInput)
	}

	thinkCtx, cancel := context.WithTimeout(ctx, activationTimeout)
	defer cancel()

	meta, _ := ctx.Value("meta").(map[string]string)
	tools := b.toolsForContext(ctx)
	executor := b.toolExecutor()

	var totalUsage llm.Usage

	for attempt := range maxThinkAttempts {
		retry := attempt > 0
		if retry {
			slog.Warn("no terminal tool call, retrying", "pkg", "brain", "attempt", attempt)
		}

		instructions, err := b.loadInstructions(b.now(), recall, retry)
		if err != nil {
			return "", totalUsage, err
		}

		actID, ch := b.llm.Complete(thinkCtx, instructions, getInput, tools, executor)
		result := <-ch

		meta["activation_id"] = actID

		totalUsage.InputTokens += result.Usage.InputTokens
		totalUsage.OutputTokens += result.Usage.OutputTokens
		totalUsage.TotalTokens += result.Usage.TotalTokens
		totalUsage.CachedTokens += result.Usage.CachedTokens
		totalUsage.ReasoningTokens += result.Usage.ReasoningTokens

		if result.Err != nil {
			return "", totalUsage, result.Err
		}

		if hasTerminalCall(result.History) {
			return result.Output, totalUsage, nil
		}
	}

	return "", totalUsage, errors.New("no terminal tool call after retries")
}

var terminalTools = map[string]bool{
	"message_reply": true,
	"message_noop":  true,
	"message_react": true,
}

func hasTerminalCall(history []llm.ToolCallRecord) bool {
	for _, h := range history {
		if terminalTools[h.Name] {
			return true
		}
	}
	return false
}
