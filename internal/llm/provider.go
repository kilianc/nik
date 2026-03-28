package llm

import "context"

type provider interface {
	setInput(content string)
	appendAssistant(text string)
	appendUser(text string)
	addToolResult(call ToolCall, output string, isError bool)
	loadHistory(messages []Message)
	conversation() []Message
	complete(ctx context.Context) (*providerResult, error)
	prune(maxPairs int) int
	reset()
	userInput() string
	fullInput() string
	setReasoningEffort(effort string)
}

type providerResult struct {
	text               string
	toolCalls          []ToolCall
	reasoningSummaries []string
	reasoningEffort    string
	incomplete         bool
	usage              Usage
	rawJSON            string
}
