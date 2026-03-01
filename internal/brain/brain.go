package brain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
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

	activeConversations *SyncSet
	activations         *SyncSet
}

// IsActive reports whether an activation with the given run ID is still alive.
func (b *Brain) IsActive(activationID string) bool {
	return b.activations.Has(activationID)
}

func New(cfg *config.Config, llmClient *llm.Client) *Brain {
	return &Brain{
		cfg:                 cfg,
		llm:                 llmClient,
		toolExec:            make(map[string]llm.ToolExecutor),
		privileged:          make(map[string]bool),
		now:                 time.Now,
		activeConversations: NewSyncSet(),
		activations:         NewSyncSet(),
	}
}

func (b *Brain) SetSoulReader(fn func(ctx context.Context) (string, error)) {
	b.soulReader = fn
}

const activationTimeout = 5 * time.Minute

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
		conversationID := output.Meta["conversation_id"]

		if conversationID != "" && !b.activeConversations.TrySet(conversationID) {
			continue
		}

		go b.activate(ctx, output)
	}
}

func (b *Brain) activate(ctx context.Context, output DataSourceOutput) {
	if output.Meta == nil {
		output.Meta = make(map[string]string)
	}

	conversationID := output.Meta["conversation_id"]
	if conversationID != "" {
		defer b.activeConversations.Delete(conversationID)
	}

	var buf [8]byte
	_, _ = rand.Read(buf[:])
	activationID := hex.EncodeToString(buf[:])

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

	instructions, err := b.loadInstructions(now())
	if err != nil {
		return "", llm.Usage{}, err
	}

	userInput := strings.Join(input, "\n")
	userInput += "\n\n---\n\nRemember to obey the json output contract."

	thinkCtx, cancel := context.WithTimeout(ctx, activationTimeout)
	defer cancel()

	meta, _ := ctx.Value("meta").(map[string]string)

	tools := b.toolsForContext(ctx)
	output, usage, toolCalls, processErr := b.llm.Complete(thinkCtx, instructions, userInput, tools, b.toolExecutor())
	b.writeDebugRecord(meta, instructions, userInput, output, tools, toolCalls, usage, processErr)

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
