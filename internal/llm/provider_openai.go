package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type openaiProvider struct {
	params      responses.ResponseNewParams
	items       responses.ResponseInputParam
	apiClient   *openai.Client
	codexClient *openai.Client
	streaming   bool
	jsonOutput  bool
}

func newOpenAIProvider(client *Client, instructions string, tools []ToolDef) *openaiProvider {
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

	return &openaiProvider{
		params:      params,
		apiClient:   client.apiClient,
		codexClient: client.codexClient,
		streaming:   client.codexClient != nil,
		jsonOutput:  client.jsonOutput,
	}
}

func (p *openaiProvider) setInput(content string) {
	content = ensureJSONInput(content, p.jsonOutput)
	msg := responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser)

	if len(p.items) == 0 {
		p.items = append(p.items, msg)
	} else {
		p.items[0] = msg
	}
}

func (p *openaiProvider) appendAssistant(text string) {
	p.items = append(p.items, responses.ResponseInputItemParamOfMessage(text, responses.EasyInputMessageRoleAssistant))
}

func (p *openaiProvider) appendUser(text string) {
	p.items = append(p.items, responses.ResponseInputItemParamOfMessage(text, responses.EasyInputMessageRoleUser))
}

func (p *openaiProvider) addToolResult(call ToolCall, output string, _ bool) {
	p.items = append(p.items, responses.ResponseInputItemParamOfFunctionCall(call.Arguments, call.CallID, call.Name))
	p.items = append(p.items, responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, output))
}

func (p *openaiProvider) conversation() []Message {
	msgs := make([]Message, 0, len(p.items))
	for _, item := range p.items {
		switch {
		case item.OfMessage != nil:
			role := string(item.OfMessage.Role)
			content := ""
			if item.OfMessage.Content.OfString.Valid() {
				content = item.OfMessage.Content.OfString.Value
			}
			msgs = append(msgs, Message{Role: role, Content: content})
		case item.OfFunctionCall != nil:
			msgs = append(msgs, Message{
				Role:    "tool_call",
				Content: item.OfFunctionCall.Arguments,
				Name:    item.OfFunctionCall.Name,
				CallID:  item.OfFunctionCall.CallID,
			})
		case item.OfFunctionCallOutput != nil:
			output := ""
			if item.OfFunctionCallOutput.Output.OfString.Valid() {
				output = item.OfFunctionCallOutput.Output.OfString.Value
			}
			msgs = append(msgs, Message{
				Role:    "tool_result",
				Content: output,
				CallID:  item.OfFunctionCallOutput.CallID,
			})
		}
	}
	return msgs
}

func (p *openaiProvider) loadHistory(messages []Message) {
	items := make(responses.ResponseInputParam, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "user":
			items = append(items, responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRoleUser))
		case "assistant":
			items = append(items, responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRoleAssistant))
		case "tool_call":
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(m.Content, m.CallID, m.Name))
		case "tool_result":
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(m.CallID, m.Content))
		}
	}
	p.items = items
}

func (p *openaiProvider) complete(ctx context.Context) (*providerResult, error) {
	p.params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: p.items}

	apiClient := p.apiClient
	if p.codexClient != nil {
		apiClient = p.codexClient
	}

	var resp *responses.Response
	var err error

	if p.streaming {
		resp, err = completeStreaming(ctx, apiClient, p.params)
	} else {
		resp, err = apiClient.Responses.New(ctx, p.params)
	}

	if err != nil {
		return nil, err
	}

	var effort string
	if e := string(resp.Reasoning.Effort); e != "" {
		effort = e
	}

	usage := Usage{
		InputTokens:     resp.Usage.InputTokens,
		OutputTokens:    resp.Usage.OutputTokens,
		TotalTokens:     resp.Usage.TotalTokens,
		CachedTokens:    resp.Usage.InputTokensDetails.CachedTokens,
		ReasoningTokens: resp.Usage.OutputTokensDetails.ReasoningTokens,
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

	result := &providerResult{
		text:               resp.OutputText(),
		reasoningSummaries: summaries,
		reasoningEffort:    effort,
		usage:              usage,
		rawJSON:            resp.RawJSON(),
		incomplete:         resp.Status == responses.ResponseStatusIncomplete,
	}

	if !result.incomplete {
		for _, item := range resp.Output {
			if item.Type != "function_call" {
				continue
			}
			fc := item.AsFunctionCall()
			result.toolCalls = append(result.toolCalls, ToolCall{
				CallID:    fc.CallID,
				Name:      fc.Name,
				Arguments: fc.Arguments,
			})
		}
	}

	return result, nil
}

func (p *openaiProvider) prune(maxPairs int) int {
	before := len(p.items)
	p.items = pruneItems(p.items, maxPairs)
	return before - len(p.items)
}

func (p *openaiProvider) userInput() string {
	if len(p.items) == 0 {
		return ""
	}
	return extractInputFromItem(p.items[0])
}

func (p *openaiProvider) fullInput() string {
	return extractInput(p.items)
}

func buildToolParams(tools []ToolDef) []responses.ToolUnionParam {
	params := make([]responses.ToolUnionParam, len(tools))

	for i, t := range tools {
		params[i] = responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  t.Parameters,
				Strict:      openai.Bool(true),
			},
		}
	}

	return params
}

func pruneItems(items responses.ResponseInputParam, maxPairs int) responses.ResponseInputParam {
	var pairCount int
	for _, item := range items[1:] {
		if item.OfFunctionCallOutput != nil {
			pairCount++
		}
	}

	if pairCount <= maxPairs {
		return items
	}

	dropPairs := pairCount - maxPairs
	var dropped int
	cutIdx := 1
	for cutIdx < len(items) && dropped < dropPairs {
		if items[cutIdx].OfFunctionCallOutput != nil {
			dropped++
		}
		cutIdx++
	}

	pruned := make(responses.ResponseInputParam, 0, len(items)-(cutIdx-1))
	pruned = append(pruned, items[0])
	pruned = append(pruned, items[cutIdx:]...)
	return pruned
}

func completeStreaming(ctx context.Context, client *openai.Client, params responses.ResponseNewParams) (*responses.Response, error) {
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
		return nil, stream.Err()
	}

	if final == nil {
		return nil, fmt.Errorf("stream ended without response.completed event")
	}

	return final, nil
}

func extractInput(items responses.ResponseInputParam) string {
	var parts []string
	for _, item := range items {
		if item.OfMessage == nil {
			continue
		}
		if !item.OfMessage.Content.OfString.Valid() {
			continue
		}
		if s := item.OfMessage.Content.OfString.Value; s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
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

func ensureJSONInput(content string, jsonOutput bool) string {
	if !jsonOutput {
		return content
	}

	if strings.TrimSpace(content) != "" {
		return content
	}

	return jsonObjectInputHint
}
