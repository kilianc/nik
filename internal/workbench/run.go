package workbench

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

func Run(ctx context.Context, run db.ExperimentVariantRun, maxRounds int, clientOpts []llm.ClientOption) (db.ExperimentVariantRun, error) {
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

	executor := func(_ context.Context, _ llm.ToolCall) (string, error) {
		return `"ok"`, nil
	}

	activation := llm.NewActivation(client, llm.NoopRecorder{}, run.Instructions, tools)
	activation.SetInput(run.UserInput)
	feedHistory(activation, run.PriorRounds, run.PriorToolCalls)

	var rounds []llm.RoundResult

	for r := range maxRounds {
		rr, err := activation.Round(ctx)
		if err != nil {
			break
		}

		rounds = append(rounds, *rr)

		terminal := len(rr.ToolCalls) == 0 || (len(rr.ToolCalls) == 1 && rr.ToolCalls[0].Name == "done")
		if terminal || r == maxRounds-1 {
			break
		}

		activation.ExecuteTools(ctx, rr, executor)
	}

	for _, rr := range rounds {
		run.InputTokens += rr.RoundUsage.InputTokens
		run.OutputTokens += rr.RoundUsage.OutputTokens
		run.CachedTokens += rr.RoundUsage.CachedTokens
		run.ReasoningTokens += rr.RoundUsage.ReasoningTokens
	}

	run.ToolCalls = marshalToolCalls(rounds)
	run.ModelOutput = lastOutput(rounds)
	run.ReasoningSummaries = marshalSummaries(rounds)

	return run, nil
}

func feedHistory(act *llm.Activation, rounds []db.ActivationRound, toolCalls []db.ToolCallListRow) {
	seq := 0
	for _, r := range rounds {
		if r.ModelOutput != "" {
			act.AppendAssistantText(r.ModelOutput)
		}

		for _, tc := range toolCalls {
			if tc.Round != r.Round {
				continue
			}

			act.AddToolResult(llm.ToolCall{
				CallID:    fmt.Sprintf("call_replay_%d", seq),
				Name:      tc.Name,
				Arguments: tc.Input,
			}, tc.Output, false)
			seq++
		}
	}
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

func lastOutput(rounds []llm.RoundResult) string {
	for i := len(rounds) - 1; i >= 0; i-- {
		if rounds[i].Text != "" {
			return rounds[i].Text
		}
	}
	return ""
}

func marshalSummaries(rounds []llm.RoundResult) string {
	var all []string
	for _, rr := range rounds {
		all = append(all, rr.ReasoningSummaries...)
	}
	return db.MarshalStringSlice(all)
}
