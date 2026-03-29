package llm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type ActivationRecorder interface {
	Start(ctx context.Context, model string)
	Round(ctx context.Context, round, attempt int, messages string, summaries []string, usage Usage) string
	ToolCall(ctx context.Context, roundID string, call ToolCall, result ExecResult)
	Sync(ctx context.Context, stats ActivationStats)
	Finish(ctx context.Context, stats ActivationStats)
}

type ActivationStats struct {
	Model           string
	ReasoningEffort string
	Verbosity       string
	Usage           Usage
	Rounds          RoundStats
	ToolCallCount   int
	DurationMS      int64
	Error           string
	Instructions    string
	Tools           []string
	ToolSchemas     []ToolDef
}

type NoopRecorder struct{}

func (NoopRecorder) Start(context.Context, string) {}
func (NoopRecorder) Round(context.Context, int, int, string, []string, Usage) string {
	return ""
}
func (NoopRecorder) ToolCall(context.Context, string, ToolCall, ExecResult) {}
func (NoopRecorder) Sync(context.Context, ActivationStats)                  {}
func (NoopRecorder) Finish(context.Context, ActivationStats)                {}

type RoundResult struct {
	Text               string
	ToolCalls          []ToolCall
	ReasoningSummaries []string
	Incomplete         bool
	RoundUsage         Usage
}

type Activation struct {
	client        *Client
	recorder      ActivationRecorder
	prov          provider
	total         Usage
	rounds        RoundStats
	extra         CompletionExtra
	history       []ToolCallRecord
	round         int
	attempt       int
	maxRounds     int
	startTime     time.Time
	prevSig       string
	repeats       int
	lastRoundID   string
	instructions  string
	toolNames     []string
	toolDefs      []ToolDef
	verbosity     string
	activationErr string
}

func NewActivation(client *Client, rec ActivationRecorder, instructions string, tools []ToolDef) *Activation {
	tools = injectReason(tools)

	var prov provider
	if client.isAnthropic() {
		prov = newAnthropicProvider(client, instructions, tools)
	} else {
		prov = newOpenAIProvider(client, instructions, tools)
	}

	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}

	var verbosity string
	if client.verbosity != nil {
		verbosity = *client.verbosity
	}

	return &Activation{
		client:       client,
		recorder:     rec,
		prov:         prov,
		startTime:    time.Now(),
		instructions: instructions,
		toolNames:    names,
		toolDefs:     tools,
		verbosity:    verbosity,
	}
}

func (s *Activation) Start(ctx context.Context) {
	s.recorder.Start(ctx, *s.client.model)
}

func (s *Activation) SetError(err error) {
	if err != nil {
		s.activationErr = err.Error()
	}
}

func (s *Activation) Close(ctx context.Context) {
	s.recorder.Finish(ctx, ActivationStats{
		Model:           *s.client.model,
		ReasoningEffort: s.extra.ReasoningEffort,
		Verbosity:       s.verbosity,
		Usage:           s.total,
		Rounds:          s.rounds,
		ToolCallCount:   len(s.history),
		DurationMS:      time.Since(s.startTime).Milliseconds(),
		Error:           s.activationErr,
		Instructions:    s.instructions,
		Tools:           s.toolNames,
		ToolSchemas:     s.toolDefs,
	})
}

func (s *Activation) SetInput(content string) {
	s.prov.setInput(content)
}

func (s *Activation) LoadHistory(messages []Message) {
	s.prov.loadHistory(messages)
}

func (s *Activation) Attempt() int { return s.attempt }

func (s *Activation) SetMaxRounds(n int) { s.maxRounds = n }

func (s *Activation) SetReasoningEffort(effort string) {
	s.prov.setReasoningEffort(effort)
}

func (s *Activation) Round(ctx context.Context) (*RoundResult, error) {
	limit := s.maxRounds
	if limit <= 0 {
		limit = defaultMaxRounds
	}
	if s.round >= limit {
		return nil, fmt.Errorf("max rounds (%d) reached without completion", limit)
	}

	pr, err := s.prov.complete(ctx)
	if err != nil {
		s.attempt++
		return nil, fmt.Errorf("round %d: %w", s.round, err)
	}

	s.extra.RawResponses = append(s.extra.RawResponses, pr.rawJSON)

	if pr.reasoningEffort != "" {
		s.extra.ReasoningEffort = pr.reasoningEffort
	}

	s.total.InputTokens += pr.usage.InputTokens
	s.total.OutputTokens += pr.usage.OutputTokens
	s.total.TotalTokens += pr.usage.TotalTokens
	s.total.CachedTokens += pr.usage.CachedTokens
	s.total.ReasoningTokens += pr.usage.ReasoningTokens

	s.rounds.RoundCount++
	if pr.usage.InputTokens > s.rounds.MaxInputTokensPerRound {
		s.rounds.MaxInputTokensPerRound = pr.usage.InputTokens
	}
	if pr.usage.TotalTokens > s.rounds.MaxTotalTokensPerRound {
		s.rounds.MaxTotalTokensPerRound = pr.usage.TotalTokens
	}

	result := &RoundResult{
		Text:               pr.text,
		ReasoningSummaries: pr.reasoningSummaries,
		RoundUsage:         pr.usage,
	}

	if pr.incomplete {
		result.Incomplete = true
		s.attempt = 0
		s.round++
		return result, nil
	}

	result.ToolCalls = pr.toolCalls

	if len(result.ToolCalls) > 0 {
		sig := roundSignature(result.ToolCalls)
		if sig == s.prevSig {
			s.repeats++
		} else {
			s.repeats = 1
		}
		s.prevSig = sig
	}

	msgs := MarshalMessages(s.prov.conversation())

	s.lastRoundID = s.recorder.Round(ctx, s.round, s.attempt, msgs, pr.reasoningSummaries, pr.usage)
	s.attempt = 0
	s.round++

	s.recorder.Sync(ctx, ActivationStats{
		Model:         *s.client.model,
		Usage:         s.total,
		Rounds:        s.rounds,
		ToolCallCount: len(s.history),
		DurationMS:    time.Since(s.startTime).Milliseconds(),
	})

	return result, nil
}

func (s *Activation) Repeats() int { return s.repeats }

type ExecResult struct {
	Output  string
	Elapsed time.Duration
	IsErr   bool
}

func (a *Activation) ExecuteTools(ctx context.Context, result *RoundResult, exec ToolExecutor) []ExecResult {
	results := make([]ExecResult, len(result.ToolCalls))

	var wg sync.WaitGroup
	wg.Add(len(result.ToolCalls))

	for i, call := range result.ToolCalls {
		go func(i int, call ToolCall) {
			defer wg.Done()
			start := time.Now()
			out, err := exec(ctx, call)
			elapsed := time.Since(start)
			if err != nil {
				results[i] = ExecResult{Output: ToolError(err), Elapsed: elapsed, IsErr: true}
				return
			}
			results[i] = ExecResult{Output: out, Elapsed: elapsed}
		}(i, call)
	}

	wg.Wait()

	if result.Text != "" {
		a.AppendAssistantText(result.Text)
	}

	for i, call := range result.ToolCalls {
		a.AddToolResult(call, results[i].Output, results[i].IsErr)
		a.recorder.ToolCall(ctx, a.lastRoundID, call, results[i])
	}

	return results
}

func (s *Activation) AddToolResult(call ToolCall, output string, isError bool) {
	rec := ToolCallRecord{
		Name:   call.Name,
		Args:   call.Arguments,
		Result: output,
		Error:  isError,
	}
	s.history = append(s.history, rec)

	s.prov.addToolResult(call, output, isError)
}

func (s *Activation) AppendAssistantText(text string) {
	if text == "" {
		return
	}
	s.prov.appendAssistant(text)
}

func (s *Activation) AppendUserMessage(text string) {
	s.prov.appendUser(text)
}

func (s *Activation) ResetConversation() {
	s.prov.reset()
}

func (s *Activation) Prune() {
	pairLimit := maxPairsForModel(*s.client.model)
	dropped := s.prov.prune(pairLimit)
	if dropped > 0 {
		slog.Info("pruned tool history", "pkg", "llm", "round", s.round, "dropped_items", dropped, "pair_limit", pairLimit)
	}
}

func (s *Activation) Usage() Usage              { return s.total }
func (s *Activation) Rounds() RoundStats        { return s.rounds }
func (s *Activation) History() []ToolCallRecord { return s.history }
func (s *Activation) Extra() CompletionExtra    { return s.extra }
func (s *Activation) RoundNumber() int          { return s.round }
func (s *Activation) StartTime() time.Time      { return s.startTime }

func (s *Activation) UserInput() string {
	return s.prov.userInput()
}

func (s *Activation) FullInput() string {
	return s.prov.fullInput()
}

func injectReason(tools []ToolDef) []ToolDef {
	out := make([]ToolDef, len(tools))

	for i, t := range tools {
		props, _ := t.Parameters["properties"].(map[string]any)

		newProps := make(map[string]any, len(props)+1)
		for k, v := range props {
			newProps[k] = v
		}
		newProps["reason"] = map[string]any{
			"type":        "string",
			"description": "Why you are calling this tool right now.",
		}

		req, _ := t.Parameters["required"].([]string)
		newReq := make([]string, len(req)+1)
		copy(newReq, req)
		newReq[len(req)] = "reason"

		newParams := make(map[string]any, len(t.Parameters))
		for k, v := range t.Parameters {
			newParams[k] = v
		}
		newParams["properties"] = newProps
		newParams["required"] = newReq

		out[i] = ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  newParams,
		}
	}

	return out
}
