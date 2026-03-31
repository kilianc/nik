package brain

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/log"
	"github.com/kciuffolo/nik/internal/prompt"
)

type Brain struct {
	cfg             *config.Config
	conn            *sql.DB
	llm             *llm.Client
	pr              *prompt.Renderer
	recorder        llm.ActivationRecorder
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

func New(cfg *config.Config, llmClient *llm.Client, pr *prompt.Renderer) *Brain {
	b := &Brain{
		cfg:        cfg,
		llm:        llmClient,
		pr:         pr,
		recorder:   llm.NoopRecorder{},
		toolExec:   make(map[string]llm.ToolExecutor),
		privileged: make(map[string]bool),
		now:        time.Now,
		claimed:    NewSyncSet(),
	}

	b.RegisterTool(llm.Tool{Def: doneToolDef, Handler: doneHandler()})

	return b
}

func (b *Brain) SetDB(conn *sql.DB) {
	b.conn = conn
}

func (b *Brain) SetRecorder(rec llm.ActivationRecorder) {
	b.recorder = rec
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
		return b.sensor.Read(ctx, convID)
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

const (
	maxAttempts   = 3
	loopThreshold = 4
)

func (b *Brain) think(ctx context.Context, getInput func() string) (_ string, _ llm.Usage, retErr error) {
	meta, _ := ctx.Value("meta").(map[string]string)
	convID := meta["conversation_id"]

	var recall string
	if b.recaller != nil && b.sensor != nil {
		recall = b.recaller(ctx, b.sensor.Peek(ctx, convID))
	}

	thinkCtx, cancel := context.WithTimeout(ctx, activationTimeout)
	defer cancel()

	tools := b.toolsForContext(ctx)
	executor := b.toolExecutor()

	instructions := b.pr.Brain(prompt.BuildBrainData(b.cfg, b.workerToolNames, b.toolDefs))

	actID := id.V7()
	meta["activation_id"] = actID

	act := llm.NewActivation(b.llm, b.recorder, instructions, tools)
	act.Start(thinkCtx)
	defer func() {
		act.SetError(retErr)
		act.Close(thinkCtx)
	}()

	id := prompt.InputData{Recall: recall, Timeline: getInput()}
	act.SetInput(b.pr.Input(id))

	var nudged bool

	for {
		result, err := act.Round(thinkCtx)
		if err != nil && llm.IsTransient(err) && act.Attempt() <= maxAttempts {
			slog.Warn("transient API error, retrying", "pkg", "brain", "attempt", act.Attempt(), "error", err)
			time.Sleep(llm.RetryDelay(act.Attempt()))
			continue
		}
		if err != nil {
			return "", act.Usage(), err
		}

		if result.Incomplete {
			return "", act.Usage(), fmt.Errorf("response incomplete in round %d", act.RoundNumber()-1)
		}

		if len(result.ToolCalls) == 0 {
			if nudged {
				return "", act.Usage(), fmt.Errorf("no done call")
			}
			nudged = true
			act.AppendAssistantText(result.Text)
			act.AppendUserMessage(b.pr.Nudge("nik-05-retry.md", struct{ Text string }{result.Text}))
			continue
		}

		if act.Repeats() >= loopThreshold {
			return "", act.Usage(), fmt.Errorf("loop: %d identical rounds calling %s", act.Repeats(), result.ToolCalls[0].Name)
		}

		for _, call := range result.ToolCalls {
			slog.Info("tool call", log.ToolCallAttrs(thinkCtx, "brain", call.Name, act.RoundNumber()-1, call.Arguments)...)
		}

		toolCallTime := time.Now().UTC()
		execResults := act.ExecuteTools(thinkCtx, result, executor)
		b.insertToolCallMessages(ctx, convID, act.RoundNumber()-1, result.ToolCalls, execResults, toolCallTime)

		if isDone(result.ToolCalls) {
			return "", act.Usage(), nil
		}

		act.ResetConversation()

		id.Timeline = getInput()
		act.SetInput(b.pr.Input(id))
	}
}

func isDone(calls []llm.ToolCall) bool {
	for _, call := range calls {
		if call.Name == doneToolName {
			return true
		}
	}

	return false
}
