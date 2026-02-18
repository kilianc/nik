package brain

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

type Brain struct {
	cfg         *config.Config
	llm         *llm.Client
	toolDefs    []llm.ToolDef
	toolExec    map[string]llm.ToolExecutor
	privileged  map[string]bool
	dataSources []DataSource
	soulReader  func(ctx context.Context) (string, error)
	now         func() time.Time

	mu     sync.Mutex
	active map[string]bool
}

func New(cfg *config.Config, llmClient *llm.Client) *Brain {
	return &Brain{
		cfg:        cfg,
		llm:        llmClient,
		toolExec:   make(map[string]llm.ToolExecutor),
		privileged: make(map[string]bool),
		now:        time.Now,
		active:     make(map[string]bool),
	}
}

func (b *Brain) SetSoulReader(fn func(ctx context.Context) (string, error)) {
	b.soulReader = fn
}

const thinkTimeout = 5 * time.Minute

// Awake starts the main loop. Brain wakes up on each tick, checks
// notifications, and processes anything new. Blocks until ctx is cancelled.
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
			b.tick(ctx)
		}
	}
}

func (b *Brain) tick(ctx context.Context) {
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
		conversationID := output.Meta["conversation_id"]

		if conversationID != "" {
			b.mu.Lock()
			if b.active[conversationID] {
				b.mu.Unlock()
				continue
			}
			b.active[conversationID] = true
			b.mu.Unlock()
		}

		go b.handleOutput(ctx, output)
	}
}

func (b *Brain) handleOutput(ctx context.Context, output DataSourceOutput) {
	conversationID := output.Meta["conversation_id"]
	if conversationID != "" {
		defer func() {
			b.mu.Lock()
			delete(b.active, conversationID)
			b.mu.Unlock()
		}()
	}

	ctx = context.WithValue(ctx, "meta", output.Meta)

	if output.Processing != nil {
		err := output.Processing(ctx)
		if err != nil {
			slog.Error("processing callback failed", "pkg", "brain", "error", err)
		}
	}

	_, _, err := b.process(ctx, output.Lines)
	if err != nil {
		slog.Error("process failed", "pkg", "brain", "error", err)
	}

	if output.Processed != nil {
		err = output.Processed(ctx)
		if err != nil {
			slog.Error("processed callback failed", "pkg", "brain", "error", err)
		}
	}
}

func (b *Brain) process(ctx context.Context, input []string) (string, llm.Usage, error) {
	now := b.now
	if now == nil {
		now = time.Now
	}

	instructions, err := b.loadInstructions(now())
	if err != nil {
		return "", llm.Usage{}, err
	}

	userInput := strings.Join(input, "\n")
	userInput += "\n\n---\n\nRemember to obey the json output contract."

	thinkCtx, cancel := context.WithTimeout(ctx, thinkTimeout)
	defer cancel()

	tools := b.toolsForContext(ctx)
	output, usage, toolCalls, processErr := b.llm.Think(thinkCtx, instructions, userInput, tools, b.toolExecutor())
	b.writeDebugRecord(instructions, userInput, output, tools, toolCalls, usage, processErr)

	if processErr != nil {
		return "", usage, processErr
	}

	err = ensureToolCalls(toolCalls)
	if err != nil {
		return "", usage, err
	}

	return output, usage, nil
}

func ensureToolCalls(calls []llm.ToolCallRecord) error {
	if len(calls) == 0 {
		return errors.New("no tool calls produced")
	}

	return nil
}
