package workbench

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

func Run(ctx context.Context, run db.ExperimentVariantRun, clientOpts []llm.ClientOption) (db.ExperimentVariantRun, error) {
	opts := append([]llm.ClientOption{}, clientOpts...)
	if run.ReasoningEffort != "" {
		opts = append(opts, llm.WithReasoningEffort(&run.ReasoningEffort))
	}
	if run.Verbosity != "" {
		opts = append(opts, llm.WithVerbosity(&run.Verbosity))
	}

	client := llm.NewClient(&run.Model, opts...)

	var tools []llm.ToolDef
	if run.ToolSchemas != "" && run.ToolSchemas != "[]" {
		err := json.Unmarshal([]byte(run.ToolSchemas), &tools)
		if err != nil {
			return run, fmt.Errorf("parse tool schemas: %w", err)
		}
	}

	messages, err := llm.UnmarshalMessages(run.Messages)
	if err != nil {
		return run, fmt.Errorf("parse messages: %w", err)
	}

	activation := llm.NewActivation(client, llm.NoopRecorder{}, run.Instructions, tools)

	if len(messages) > 0 {
		activation.SetInput(messages[0].Content)
	}

	rr, err := activation.Round(ctx)
	if err != nil {
		return run, fmt.Errorf("round: %w", err)
	}

	run.InputTokens = rr.RoundUsage.InputTokens
	run.OutputTokens = rr.RoundUsage.OutputTokens
	run.CachedTokens = rr.RoundUsage.CachedTokens
	run.ReasoningTokens = rr.RoundUsage.ReasoningTokens

	run.ToolCalls = marshalToolCalls([]llm.RoundResult{*rr})
	run.ModelOutput = rr.Text
	run.ReasoningSummaries = marshalSummaries([]llm.RoundResult{*rr})

	return run, nil
}

func marshalToolCalls(rounds []llm.RoundResult) string {
	type tc struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}

	var calls []tc
	for _, rr := range rounds {
		for _, c := range rr.ToolCalls {
			calls = append(calls, tc{Name: c.Name, Arguments: c.Arguments})
		}
	}

	data, _ := json.Marshal(calls)
	return string(data)
}

func marshalSummaries(rounds []llm.RoundResult) string {
	var all []string
	for _, rr := range rounds {
		all = append(all, rr.ReasoningSummaries...)
	}
	return db.MarshalStringSlice(all)
}
