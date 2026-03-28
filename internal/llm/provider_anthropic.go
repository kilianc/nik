package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
)

const defaultAnthropicMaxTokens = 16384

type anthropicProvider struct {
	client   *anthropic.Client
	params   anthropic.MessageNewParams
	messages []anthropic.MessageParam

	// tracks the last assistant response so tool results can reference it
	lastResponse   *anthropic.Message
	pendingResults []anthropic.ContentBlockParamUnion
	jsonOutput     bool
}

func newAnthropicProvider(client *Client, instructions string, tools []ToolDef) *anthropicProvider {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(*client.model),
		MaxTokens: defaultAnthropicMaxTokens,
		System: []anthropic.TextBlockParam{
			{Text: instructions},
		},
		Tools: buildAnthropicTools(tools),
	}

	if client.reasoningEffort != nil && *client.reasoningEffort != "" {
		budget := thinkingBudget(*client.reasoningEffort)
		if budget > 0 {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
			if budget+1024 > params.MaxTokens {
				params.MaxTokens = budget + 8192
			}
		}
	}

	return &anthropicProvider{
		client:     client.anthropicClient,
		params:     params,
		jsonOutput: client.jsonOutput,
	}
}

func (p *anthropicProvider) setInput(content string) {
	content = ensureJSONInput(content, p.jsonOutput)
	msg := anthropic.NewUserMessage(anthropic.NewTextBlock(content))

	if len(p.messages) == 0 {
		p.messages = append(p.messages, msg)
	} else {
		p.messages[0] = msg
	}
}

func (p *anthropicProvider) appendAssistant(text string) {
	if p.lastResponse != nil && hasToolUse(p.lastResponse) {
		return
	}

	if p.lastResponse != nil {
		p.messages = append(p.messages, p.lastResponse.ToParam())
		p.lastResponse = nil
		return
	}

	p.messages = append(p.messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(text)))
}

func (p *anthropicProvider) appendUser(text string) {
	p.messages = append(p.messages, anthropic.NewUserMessage(anthropic.NewTextBlock(text)))
}

func (p *anthropicProvider) addToolResult(call ToolCall, output string, isError bool) {
	if p.lastResponse != nil {
		p.messages = append(p.messages, p.lastResponse.ToParam())
		p.lastResponse = nil
	}

	p.pendingResults = append(p.pendingResults, anthropic.NewToolResultBlock(call.CallID, output, isError))
}

func (p *anthropicProvider) conversation() []Message {
	var msgs []Message
	for _, msg := range p.messages {
		for _, block := range msg.Content {
			switch {
			case block.OfText != nil:
				msgs = append(msgs, Message{Role: string(msg.Role), Content: block.OfText.Text})
			case block.OfToolUse != nil:
				args := "{}"
				if block.OfToolUse.Input != nil {
					if data, err := json.Marshal(block.OfToolUse.Input); err == nil {
						args = string(data)
					}
				}
				msgs = append(msgs, Message{
					Role:    "tool_call",
					Content: args,
					Name:    block.OfToolUse.Name,
					CallID:  block.OfToolUse.ID,
				})
			case block.OfToolResult != nil:
				content := ""
				for _, c := range block.OfToolResult.Content {
					if c.OfText != nil {
						content = c.OfText.Text
					}
				}
				msgs = append(msgs, Message{
					Role:    "tool_result",
					Content: content,
					CallID:  block.OfToolResult.ToolUseID,
				})
			}
		}
	}
	return msgs
}

func (p *anthropicProvider) loadHistory(messages []Message) {
	var msgs []anthropic.MessageParam
	var toolUseBlocks []anthropic.ContentBlockParamUnion
	var toolResultBlocks []anthropic.ContentBlockParamUnion

	flush := func() {
		if len(toolUseBlocks) > 0 {
			msgs = append(msgs, anthropic.NewAssistantMessage(toolUseBlocks...))
			toolUseBlocks = nil
		}
		if len(toolResultBlocks) > 0 {
			msgs = append(msgs, anthropic.NewUserMessage(toolResultBlocks...))
			toolResultBlocks = nil
		}
	}

	for _, m := range messages {
		switch m.Role {
		case "user":
			flush()
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			flush()
			msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		case "tool_call":
			if len(toolResultBlocks) > 0 {
				flush()
			}
			toolUseBlocks = append(toolUseBlocks, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    m.CallID,
					Name:  m.Name,
					Input: json.RawMessage(m.Content),
				},
			})
		case "tool_result":
			if len(toolUseBlocks) > 0 && len(toolResultBlocks) == 0 {
				msgs = append(msgs, anthropic.NewAssistantMessage(toolUseBlocks...))
				toolUseBlocks = nil
			}
			toolResultBlocks = append(toolResultBlocks, anthropic.NewToolResultBlock(m.CallID, m.Content, false))
		}
	}
	flush()

	p.messages = msgs
	p.lastResponse = nil
	p.pendingResults = nil
}

func (p *anthropicProvider) complete(ctx context.Context) (*providerResult, error) {
	if len(p.pendingResults) > 0 {
		p.messages = append(p.messages, anthropic.NewUserMessage(p.pendingResults...))
		p.pendingResults = nil
	}

	p.params.Messages = p.messages

	resp, err := completeAnthropicStreaming(ctx, p.client, p.params)
	if err != nil {
		return nil, err
	}

	p.lastResponse = resp

	usage := Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CacheReadInputTokens,
	}

	var text strings.Builder
	var summaries []string
	var toolCalls []ToolCall

	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(v.Text)

		case anthropic.ThinkingBlock:
			if v.Thinking != "" {
				summaries = append(summaries, v.Thinking)
			}

		case anthropic.ToolUseBlock:
			args := string(v.Input)
			toolCalls = append(toolCalls, ToolCall{
				CallID:    v.ID,
				Name:      v.Name,
				Arguments: args,
			})
		}
	}

	result := &providerResult{
		text:               text.String(),
		toolCalls:          toolCalls,
		reasoningSummaries: summaries,
		usage:              usage,
		rawJSON:            resp.RawJSON(),
		incomplete:         resp.StopReason == anthropic.StopReasonMaxTokens,
	}

	return result, nil
}

func (p *anthropicProvider) reset() {
	p.messages = nil
	p.lastResponse = nil
	p.pendingResults = nil
}

func (p *anthropicProvider) prune(maxPairs int) int {
	if len(p.messages) <= 1 {
		return 0
	}

	var pairCount int
	for i := 1; i < len(p.messages); i += 2 {
		pairCount++
	}

	if pairCount <= maxPairs {
		return 0
	}

	dropPairs := pairCount - maxPairs
	dropItems := dropPairs * 2

	if dropItems >= len(p.messages)-1 {
		dropItems = len(p.messages) - 1
	}

	pruned := make([]anthropic.MessageParam, 0, len(p.messages)-dropItems)
	pruned = append(pruned, p.messages[0])
	pruned = append(pruned, p.messages[1+dropItems:]...)
	dropped := len(p.messages) - len(pruned)
	p.messages = pruned
	return dropped
}

func (p *anthropicProvider) userInput() string {
	if len(p.messages) == 0 {
		return ""
	}
	return extractAnthropicText(p.messages[0])
}

func (p *anthropicProvider) fullInput() string {
	var parts []string
	for _, msg := range p.messages {
		if msg.Role != "user" {
			continue
		}
		if s := extractAnthropicText(msg); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (p *anthropicProvider) setReasoningEffort(effort string) {
	if effort == "" {
		return
	}
	budget := thinkingBudget(effort)
	if budget > 0 {
		p.params.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
		if budget+1024 > p.params.MaxTokens {
			p.params.MaxTokens = budget + 8192
		}
	}
}

func extractAnthropicText(msg anthropic.MessageParam) string {
	var parts []string
	for _, block := range msg.Content {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func hasToolUse(msg *anthropic.Message) bool {
	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
}

func buildAnthropicTools(tools []ToolDef) []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, len(tools))

	for i, t := range tools {
		schema := anthropic.ToolInputSchemaParam{
			Properties: t.Parameters["properties"],
		}

		if req, ok := t.Parameters["required"]; ok {
			if reqSlice, ok := req.([]string); ok {
				schema.Required = reqSlice
			} else if reqAny, ok := req.([]any); ok {
				for _, v := range reqAny {
					if s, ok := v.(string); ok {
						schema.Required = append(schema.Required, s)
					}
				}
			}
		}

		params[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropicparam.NewOpt(t.Description),
				InputSchema: schema,
			},
		}
	}

	return params
}

func completeAnthropicStreaming(ctx context.Context, client *anthropic.Client, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	stream := client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	var msg anthropic.Message
	for stream.Next() {
		evt := stream.Current()
		err := msg.Accumulate(evt)
		if err != nil {
			return nil, fmt.Errorf("accumulate stream event: %w", err)
		}
	}

	if stream.Err() != nil {
		return nil, stream.Err()
	}

	return &msg, nil
}

func thinkingBudget(effort string) int64 {
	switch effort {
	case "low", "minimal":
		return 4096
	case "medium":
		return 8192
	case "high":
		return 16384
	case "xhigh":
		return 32768
	default:
		return 0
	}
}
