package workbench

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kciuffolo/nik/internal/db"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type Patch struct {
	File string `json:"file"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

type ReplayResult struct {
	ToolCalls          []ToolCall
	ModelOutput        string
	ReasoningSummaries []string
	InputTokens        int64
	OutputTokens       int64
	CachedTokens       int64
	ReasoningTokens    int64
}

type ToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCallKey string

func (r ReplayResult) Key() ToolCallKey {
	if len(r.ToolCalls) == 0 {
		return "no_tools"
	}
	var s string
	for i, tc := range r.ToolCalls {
		if i > 0 {
			s += "+"
		}
		s += tc.Name
	}
	return ToolCallKey(s)
}

type RunReplayParams struct {
	ActivationRoundID string
	VariantID         string
	Desired           string
	N                 int
	EffortOverride    string
}

type RunReplayResult struct {
	Attempts []AttemptResult
	Dist     []DistEntry
	Desired  string
	Errors   []string
}

type AttemptResult struct {
	Result    ReplayResult
	Key       ToolCallKey
	IsDesired bool
}

type DistEntry struct {
	Key       string  `json:"key"`
	Count     int     `json:"count"`
	Percent   float64 `json:"percent"`
	IsDesired bool    `json:"is_desired"`
}

type toolSchema struct {
	Name        string         `json:"Name"`
	Description string         `json:"Description"`
	Parameters  map[string]any `json:"Parameters"`
}

type replayJSONOutput struct {
	Attempts     []replayJSONAttempt `json:"attempts"`
	Distribution []DistEntry         `json:"distribution"`
	DesiredKey   string              `json:"desired_key,omitempty"`
}

type replayJSONAttempt struct {
	Tools           []ToolCall `json:"tools"`
	ModelOutput     string     `json:"model_output,omitempty"`
	Reasoning       []string   `json:"reasoning_summaries,omitempty"`
	Key             string     `json:"key"`
	IsDesired       bool       `json:"is_desired"`
	InputTokens     int64      `json:"input_tokens"`
	OutputTokens    int64      `json:"output_tokens"`
	CachedTokens    int64      `json:"cached_tokens"`
	ReasoningTokens int64      `json:"reasoning_tokens"`
}

func (r RunReplayResult) JSON() string {
	var attempts []replayJSONAttempt
	for _, a := range r.Attempts {
		attempts = append(attempts, replayJSONAttempt{
			Tools:           a.Result.ToolCalls,
			ModelOutput:     a.Result.ModelOutput,
			Reasoning:       a.Result.ReasoningSummaries,
			Key:             string(a.Key),
			IsDesired:       a.IsDesired,
			InputTokens:     a.Result.InputTokens,
			OutputTokens:    a.Result.OutputTokens,
			CachedTokens:    a.Result.CachedTokens,
			ReasoningTokens: a.Result.ReasoningTokens,
		})
	}

	out := replayJSONOutput{
		Attempts:     attempts,
		Distribution: r.Dist,
		DesiredKey:   r.Desired,
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data)
}

func (r RunReplayResult) Text() string {
	var b strings.Builder

	for i, a := range r.Attempts {
		tag := ""
		if a.IsDesired {
			tag = " (desired)"
		}
		fmt.Fprintf(&b, "  attempt %d: %s%s\n", i+1, a.Key, tag)
	}

	if len(r.Dist) > 0 && len(r.Attempts) > 1 {
		fmt.Fprintf(&b, "\nDISTRIBUTION:\n")
		total := len(r.Attempts)
		for _, d := range r.Dist {
			tag := ""
			if d.IsDesired {
				tag = " <- desired"
			}
			fmt.Fprintf(&b, "  %s: %d/%d (%.0f%%)%s\n", d.Key, d.Count, total, d.Percent, tag)
		}
	}

	return b.String()
}

func RunReplay(ctx context.Context, conn *sql.DB, client *openai.Client, p RunReplayParams) (RunReplayResult, error) {
	var patches []Patch
	if p.VariantID != "" {
		var err error
		patches, err = VariantPatches(ctx, conn, p.VariantID)
		if err != nil {
			return RunReplayResult{}, fmt.Errorf("load variant patches: %w", err)
		}
	}

	round, err := db.ActivationRoundGet(ctx, conn, p.ActivationRoundID)
	if err != nil {
		return RunReplayResult{}, err
	}

	act, err := db.ActivationGet(ctx, conn, round.ActivationID)
	if err != nil {
		return RunReplayResult{}, err
	}

	priorRounds, err := db.ActivationRoundList(ctx, conn, round.ActivationID, &round.Round)
	if err != nil {
		return RunReplayResult{}, err
	}

	allToolCalls, err := db.ToolCallList(ctx, conn, round.ActivationID, nil)
	if err != nil {
		return RunReplayResult{}, err
	}

	var maxRound int
	if len(priorRounds) > 0 {
		maxRound = priorRounds[len(priorRounds)-1].Round
	}

	var priorToolCalls []db.ToolCallListRow
	for _, tc := range allToolCalls {
		if tc.Round <= maxRound {
			priorToolCalls = append(priorToolCalls, tc)
		}
	}

	instructions, err := applyPatches(act.Instructions, patches)
	if err != nil {
		return RunReplayResult{}, fmt.Errorf("apply patches: %w", err)
	}

	items := buildItems(round, priorRounds, priorToolCalls)

	tools, err := parseToolSchemas(act.ToolSchemas)
	if err != nil {
		return RunReplayResult{}, fmt.Errorf("parse tool schemas: %w", err)
	}

	effort := act.ReasoningEffort
	if p.EffortOverride != "" {
		effort = p.EffortOverride
	}

	result := RunReplayResult{Desired: p.Desired}

	for i := range p.N {
		rr, err := callAPI(ctx, client, act.Model, instructions, items, tools, effort, act.Verbosity)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("attempt %d: %v", i+1, err))
			continue
		}

		key := rr.Key()
		isDesired := p.Desired != "" && string(key) == p.Desired

		if p.VariantID != "" {
			_, err = RecordRun(ctx, conn, p.VariantID, rr, isDesired)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("record run %d: %v", i+1, err))
			}
		}

		result.Attempts = append(result.Attempts, AttemptResult{
			Result:    rr,
			Key:       key,
			IsDesired: isDesired,
		})
	}

	result.Dist = computeDistribution(result.Attempts, p.Desired)
	return result, nil
}

func buildItems(target db.ActivationRound, priorRounds []db.ActivationRound, priorToolCalls []db.ToolCallListRow) responses.ResponseInputParam {
	items := responses.ResponseInputParam{
		responses.ResponseInputItemParamOfMessage(target.UserInput, responses.EasyInputMessageRoleUser),
	}

	callSeq := 0

	for _, r := range priorRounds {
		if r.ModelOutput != "" {
			items = append(items, responses.ResponseInputItemParamOfMessage(r.ModelOutput, responses.EasyInputMessageRoleAssistant))
		}

		for _, tc := range priorToolCalls {
			if tc.Round != r.Round {
				continue
			}

			callID := fmt.Sprintf("call_replay_%d", callSeq)
			callSeq++

			items = append(items, responses.ResponseInputItemParamOfFunctionCall(tc.Input, callID, tc.Name))
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(callID, tc.Output))
		}
	}

	return items
}

func parseToolSchemas(raw string) ([]toolSchema, error) {
	if raw == "" || raw == "[]" {
		return nil, nil
	}

	var schemas []toolSchema

	err := json.Unmarshal([]byte(raw), &schemas)
	if err != nil {
		return nil, fmt.Errorf("unmarshal tool schemas: %w", err)
	}

	return schemas, nil
}

func applyPatches(instructions string, patches []Patch) (string, error) {
	for _, p := range patches {
		if !strings.Contains(instructions, p.Old) {
			return "", fmt.Errorf("patch for %s: old text not found in instructions", p.File)
		}

		instructions = strings.Replace(instructions, p.Old, p.New, 1)
	}

	return instructions, nil
}

func callAPI(ctx context.Context, client *openai.Client, model, instructions string, items responses.ResponseInputParam, tools []toolSchema, effort, verbosity string) (ReplayResult, error) {
	toolParams := make([]responses.ToolUnionParam, len(tools))

	for i, t := range tools {
		toolParams[i] = responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  t.Parameters,
				Strict:      openai.Bool(true),
			},
		}
	}

	params := responses.ResponseNewParams{
		Model:        shared.ResponsesModel(model),
		Instructions: openai.String(instructions),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: items,
		},
		Tools: toolParams,
		Store: openai.Bool(false),
		Reasoning: shared.ReasoningParam{
			Summary: shared.ReasoningSummaryDetailed,
		},
	}

	if effort != "" {
		params.Reasoning.Effort = shared.ReasoningEffort(effort)
	}

	if verbosity != "" {
		params.Text = responses.ResponseTextConfigParam{
			Verbosity: responses.ResponseTextConfigVerbosity(verbosity),
		}
	}

	stream := client.Responses.NewStreaming(ctx, params)
	defer stream.Close()

	var final *responses.Response

	for stream.Next() {
		evt := stream.Current()
		completed := evt.AsResponseCompleted()
		if completed.Type == "response.completed" {
			final = &completed.Response
		}
	}

	if stream.Err() != nil {
		return ReplayResult{}, fmt.Errorf("api stream: %w", stream.Err())
	}

	if final == nil {
		return ReplayResult{}, fmt.Errorf("stream ended without response.completed event")
	}

	return extractResult(final), nil
}

func extractResult(resp *responses.Response) ReplayResult {
	result := ReplayResult{
		ModelOutput:     resp.OutputText(),
		InputTokens:     int64(resp.Usage.InputTokens),
		OutputTokens:    int64(resp.Usage.OutputTokens),
		CachedTokens:    int64(resp.Usage.InputTokensDetails.CachedTokens),
		ReasoningTokens: int64(resp.Usage.OutputTokensDetails.ReasoningTokens),
	}

	for _, item := range resp.Output {
		if item.Type == "function_call" {
			fc := item.AsFunctionCall()
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				Name:      fc.Name,
				Arguments: fc.Arguments,
			})
		}

		if item.Type == "reasoning" {
			for _, s := range item.AsReasoning().Summary {
				if s.Text != "" {
					result.ReasoningSummaries = append(result.ReasoningSummaries, s.Text)
				}
			}
		}
	}

	return result
}

func ParsePatches(raw string) ([]Patch, error) {
	if raw == "" || raw == "[]" {
		return nil, nil
	}

	var patches []Patch

	err := json.Unmarshal([]byte(raw), &patches)
	if err != nil {
		return nil, fmt.Errorf("unmarshal patches: %w", err)
	}

	return patches, nil
}

func computeDistribution(attempts []AttemptResult, desired string) []DistEntry {
	counts := map[string]int{}
	for _, a := range attempts {
		counts[string(a.Key)]++
	}

	total := len(attempts)
	var dist []DistEntry

	for k, v := range counts {
		pct := 0.0
		if total > 0 {
			pct = float64(v) / float64(total) * 100
		}

		dist = append(dist, DistEntry{
			Key:       k,
			Count:     v,
			Percent:   pct,
			IsDesired: desired != "" && k == desired,
		})
	}

	return dist
}
