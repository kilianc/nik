package llm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type ActivationRecorder interface {
	Start(ctx context.Context, model string)
	Round(ctx context.Context, round, attempt int, input, output string, summaries []string) string
	ToolCall(ctx context.Context, roundID string, call ToolCall, result ExecResult)
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
	Instructions    string
	Tools           []string
}

type NoopRecorder struct{}

func (NoopRecorder) Start(context.Context, string)                                    {}
func (NoopRecorder) Round(context.Context, int, int, string, string, []string) string { return "" }
func (NoopRecorder) ToolCall(context.Context, string, ToolCall, ExecResult)           {}
func (NoopRecorder) Finish(context.Context, ActivationStats)                          {}

type RoundResult struct {
	Text               string
	ToolCalls          []ToolCall
	ReasoningSummaries []string
	Incomplete         bool
	RoundUsage         Usage
}

type Activation struct {
	client       *Client
	recorder     ActivationRecorder
	params       responses.ResponseNewParams
	items        responses.ResponseInputParam
	useStreaming bool
	total        Usage
	rounds       RoundStats
	extra        CompletionExtra
	history      []ToolCallRecord
	round        int
	attempt      int
	startTime    time.Time
	prevSig      string
	repeats      int
	lastRoundID  string
	instructions string
	toolNames    []string
	verbosity    string
}

func NewActivation(client *Client, rec ActivationRecorder, instructions string, tools []ToolDef) *Activation {
	params := responses.ResponseNewParams{
		Model:        shared.ResponsesModel(*client.model),
		Instructions: openai.String(instructions),
		Tools:        buildToolParams(tools),
		Reasoning: shared.ReasoningParam{
			Summary: shared.ReasoningSummaryDetailed,
		},
	}

	if client.reasoningEffort != nil && *client.reasoningEffort != "" {
		params.Reasoning.Effort = shared.ReasoningEffort(*client.reasoningEffort)
	}

	if client.verbosity != nil && *client.verbosity != "" {
		params.Text = responses.ResponseTextConfigParam{
			Verbosity: responses.ResponseTextConfigVerbosity(*client.verbosity),
		}
	}

	if client.jsonOutput {
		params.Text.Format = responses.ResponseFormatTextConfigUnionParam{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	}

	m := *client.model
	noReasoning := (strings.Contains(m, "spark") || strings.Contains(m, "nano") || strings.Contains(m, "4.1-mini")) && !strings.Contains(m, "5.4")
	if noReasoning {
		params.Reasoning = shared.ReasoningParam{}
		params.Text = responses.ResponseTextConfigParam{}
	}

	if client.codexClient != nil {
		params.Store = openai.Bool(false)
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
		params:       params,
		useStreaming: client.codexClient != nil,
		startTime:    time.Now(),
		instructions: instructions,
		toolNames:    names,
		verbosity:    verbosity,
	}
}

func (s *Activation) Start(ctx context.Context) {
	s.recorder.Start(ctx, *s.client.model)
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
		Instructions:    s.instructions,
		Tools:           s.toolNames,
	})
}

func (s *Activation) SetInput(content string) {
	content = ensureJSONInput(content, s.client.jsonOutput)
	msg := responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser)

	if len(s.items) == 0 {
		s.items = append(s.items, msg)
	} else {
		s.items[0] = msg
	}
}

func (s *Activation) Attempt() int { return s.attempt }

func (s *Activation) Round(ctx context.Context) (*RoundResult, error) {
	if s.round >= maxRounds {
		return nil, fmt.Errorf("max rounds (%d) reached without completion", maxRounds)
	}

	s.params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: s.items}

	apiClient := s.client.apiClient
	if s.client.codexClient != nil {
		apiClient = s.client.codexClient
	}

	var resp *responses.Response
	var err error

	if s.useStreaming {
		resp, err = completeStreaming(ctx, apiClient, s.params)
	} else {
		resp, err = apiClient.Responses.New(ctx, s.params)
	}

	if err != nil {
		s.attempt++
		return nil, fmt.Errorf("round %d: %w", s.round, err)
	}

	s.extra.RawResponses = append(s.extra.RawResponses, resp.RawJSON())

	if effort := string(resp.Reasoning.Effort); effort != "" {
		s.extra.ReasoningEffort = effort
	}

	s.total.InputTokens += resp.Usage.InputTokens
	s.total.OutputTokens += resp.Usage.OutputTokens
	s.total.TotalTokens += resp.Usage.TotalTokens
	s.total.CachedTokens += resp.Usage.InputTokensDetails.CachedTokens
	s.total.ReasoningTokens += resp.Usage.OutputTokensDetails.ReasoningTokens

	s.rounds.RoundCount++
	if resp.Usage.InputTokens > s.rounds.MaxInputTokensPerRound {
		s.rounds.MaxInputTokensPerRound = resp.Usage.InputTokens
	}
	if resp.Usage.TotalTokens > s.rounds.MaxTotalTokensPerRound {
		s.rounds.MaxTotalTokensPerRound = resp.Usage.TotalTokens
	}

	var summaries []string
	for _, item := range resp.Output {
		if item.Type != "reasoning" {
			continue
		}
		for _, su := range item.AsReasoning().Summary {
			if su.Text != "" {
				summaries = append(summaries, su.Text)
			}
		}
	}

	result := &RoundResult{
		Text:               resp.OutputText(),
		ReasoningSummaries: summaries,
		RoundUsage: Usage{
			InputTokens:     resp.Usage.InputTokens,
			OutputTokens:    resp.Usage.OutputTokens,
			TotalTokens:     resp.Usage.TotalTokens,
			CachedTokens:    resp.Usage.InputTokensDetails.CachedTokens,
			ReasoningTokens: resp.Usage.OutputTokensDetails.ReasoningTokens,
		},
	}

	if resp.Status == responses.ResponseStatusIncomplete {
		result.Incomplete = true
		s.attempt = 0
		s.round++
		return result, nil
	}

	for _, item := range resp.Output {
		if item.Type != "function_call" {
			continue
		}
		fc := item.AsFunctionCall()
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			CallID:    fc.CallID,
			Name:      fc.Name,
			Arguments: fc.Arguments,
		})
	}

	if len(result.ToolCalls) > 0 {
		sig := roundSignature(result.ToolCalls)
		if sig == s.prevSig {
			s.repeats++
		} else {
			s.repeats = 1
		}
		s.prevSig = sig
	}

	s.lastRoundID = s.recorder.Round(ctx, s.round, s.attempt, s.UserInput(), result.Text, summaries)
	s.attempt = 0
	s.round++
	return result, nil
}

func (s *Activation) Repeats() int { return s.repeats }

type ExecResult struct {
	Output  string
	Elapsed time.Duration
	IsErr   bool
}

func (s *Activation) ExecuteTools(ctx context.Context, result *RoundResult, exec ToolExecutor) []ExecResult {
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
		s.AppendAssistantText(result.Text)
	}

	for i, call := range result.ToolCalls {
		s.AddToolResult(call, results[i].Output, results[i].IsErr)
		s.recorder.ToolCall(ctx, s.lastRoundID, call, results[i])
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

	s.items = append(s.items, responses.ResponseInputItemParamOfFunctionCall(call.Arguments, call.CallID, call.Name))
	s.items = append(s.items, responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, output))
}

func (s *Activation) AppendAssistantText(text string) {
	if text == "" {
		return
	}
	s.items = append(s.items, responses.ResponseInputItemParamOfMessage(text, responses.EasyInputMessageRoleAssistant))
}

func (s *Activation) AppendUserMessage(text string) {
	s.items = append(s.items, responses.ResponseInputItemParamOfMessage(text, responses.EasyInputMessageRoleUser))
}

func (s *Activation) Prune() {
	pairLimit := maxPairsForModel(*s.client.model)
	before := len(s.items)
	s.items = pruneItems(s.items, pairLimit)
	if len(s.items) < before {
		slog.Info("pruned tool history", "pkg", "llm", "round", s.round, "dropped_items", before-len(s.items), "pair_limit", pairLimit)
	}
}

func (s *Activation) Usage() Usage              { return s.total }
func (s *Activation) Rounds() RoundStats        { return s.rounds }
func (s *Activation) History() []ToolCallRecord { return s.history }
func (s *Activation) Extra() CompletionExtra    { return s.extra }
func (s *Activation) RoundNumber() int          { return s.round }
func (s *Activation) StartTime() time.Time      { return s.startTime }

func (s *Activation) UserInput() string {
	if len(s.items) == 0 {
		return ""
	}
	return extractInputFromItem(s.items[0])
}

func (s *Activation) FullInput() string {
	return extractInput(s.items)
}

func extractInputFromItem(item responses.ResponseInputItemUnionParam) string {
	if item.OfMessage == nil {
		return ""
	}
	if !item.OfMessage.Content.OfString.Valid() {
		return ""
	}
	return item.OfMessage.Content.OfString.Value
}
