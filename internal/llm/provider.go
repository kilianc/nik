package llm

import "context"

type provider interface {
	setInput(content string)
	appendAssistant(text string)
	appendUser(text string)
	addToolResult(call ToolCall, output string, isError bool)
	complete(ctx context.Context) (*providerResult, error)
	prune(maxPairs int) int
	userInput() string
	fullInput() string
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
