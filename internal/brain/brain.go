package brain

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

type Brain struct {
	cfg           *config.Config
	llm           *llm.Client
	toolDefs      []llm.ToolDef
	toolExec      map[string]llm.ToolExecutor
	privileged    map[string]bool
	dataSources   []DataSource
	soulReader    func(ctx context.Context) (string, error)
	crewReader    func(ctx context.Context) (string, error)
	toolReactor   ToolReactor
	toolEmojis    map[string]string
	debugRecorder DebugRecorder
	now           func() time.Time

	claimed *SyncSet
}

func New(cfg *config.Config, llmClient *llm.Client) *Brain {
	return &Brain{
		cfg:        cfg,
		llm:        llmClient,
		toolExec:   make(map[string]llm.ToolExecutor),
		privileged: make(map[string]bool),
		now:        time.Now,
		claimed:    NewSyncSet(),
	}
}

func (b *Brain) SetSoulReader(fn func(ctx context.Context) (string, error)) {
	b.soulReader = fn
}

func (b *Brain) SetCrewReader(fn func(ctx context.Context) (string, error)) {
	b.crewReader = fn
}

func (b *Brain) SetToolReactor(emojis map[string]string, fn ToolReactor) {
	b.toolEmojis = emojis
	b.toolReactor = fn
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

	ctx = context.WithValue(ctx, "meta", output.Meta)

	if b.toolReactor != nil {
		reactTo := output.Meta["react_to_message_id"]
		if reactTo != "" {
			q := startReactionQueue(ctx, reactTo, b.toolReactor)
			ctx = context.WithValue(ctx, reactionQueueKey{}, q)
			defer q.close()
		}
	}

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

func (b *Brain) think(ctx context.Context, input []string) (string, llm.Usage, error) {
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

	instructions, err := b.loadInstructions(now())
	if err != nil {
		return "", llm.Usage{}, err
	}

	actID, ch := b.llm.Complete(thinkCtx, instructions, userInput, tools, executor)
	result := <-ch

	meta["activation_id"] = actID

	if b.debugRecorder != nil {
		b.debugRecorder(DebugInput{
			Meta:         meta,
			Instructions: instructions,
			UserInput:    userInput,
			RawOutput:    result.Output,
			Tools:        tools,
			ToolCalls:    result.History,
			Extra:        result.Extra,
			Usage:        result.Usage,
			ProcessErr:   result.Err,
		})
	}

	return result.Output, result.Usage, result.Err
}
