package llm

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

func TestAnthropicProviderSetInput(t *testing.T) {
	model := "claude-opus-4-6"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)

	p.setInput("hello")
	if got := p.userInput(); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}

	p.setInput("updated")
	if got := p.userInput(); got != "updated" {
		t.Fatalf("expected 'updated', got %q", got)
	}

	if len(p.messages) != 1 {
		t.Fatalf("expected 1 message after replace, got %d", len(p.messages))
	}
}

func TestAnthropicProviderPromptCaching(t *testing.T) {
	model := "claude-opus-4-7"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)

	if len(p.params.System) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(p.params.System))
	}
	if got := string(p.params.System[0].CacheControl.Type); got != "ephemeral" {
		t.Fatalf("expected ephemeral cache control on system block, got %q", got)
	}
}

func TestAnthropicProviderEmptyInput(t *testing.T) {
	model := "claude-opus-4-7"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)

	p.setInput("")

	if len(p.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(p.messages))
	}
	if got := p.userInput(); got != anthropicEmptyTextPlaceholder {
		t.Fatalf("expected empty input to become %q, got %q", anthropicEmptyTextPlaceholder, got)
	}
}

func TestAnthropicProviderAppendMessages(t *testing.T) {
	model := "claude-opus-4-6"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)
	p.setInput("test")

	p.appendAssistant("thinking")
	p.appendUser("nudge")

	if len(p.messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(p.messages))
	}

	if p.messages[0].Role != "user" {
		t.Fatalf("expected first message to be user, got %s", p.messages[0].Role)
	}
	if p.messages[1].Role != "assistant" {
		t.Fatalf("expected second message to be assistant, got %s", p.messages[1].Role)
	}
	if p.messages[2].Role != "user" {
		t.Fatalf("expected third message to be user, got %s", p.messages[2].Role)
	}
}

func TestAnthropicProviderToolResultAccumulation(t *testing.T) {
	model := "claude-opus-4-6"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)
	p.setInput("test")

	call1 := ToolCall{CallID: "t1", Name: "db_query", Arguments: `{}`}
	call2 := ToolCall{CallID: "t2", Name: "fs_read", Arguments: `{}`}

	p.addToolResult(call1, "result1", false)
	p.addToolResult(call2, "result2", false)

	if len(p.pendingResults) != 2 {
		t.Fatalf("expected 2 pending results, got %d", len(p.pendingResults))
	}
}

// Regression: a user message appended mid-loop (e.g. the task runner's
// 5-minute "report" nudge) while a round's tool results are still pending must
// not land between an assistant tool_use and its tool_result. Anthropic rejects
// that with "tool_use ids without tool_result in the next message" (400).
func TestAnthropicProviderAppendUserKeepsToolResultAdjacent(t *testing.T) {
	model := "claude-opus-4-7"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)
	p.setInput("do work")

	// End-of-round state: the assistant tool_use turn is in the message list
	// and its result is pending (not yet flushed) — what addToolResult leaves
	// behind between rounds.
	p.messages = append(p.messages, anthropic.NewAssistantMessage(anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{ID: "tool1", Name: "shell"},
	}))
	p.pendingResults = append(p.pendingResults, anthropic.NewToolResultBlock("tool1", "running", false))

	// Runner injects a user message before the next round.
	p.appendUser("You haven't reported in 5 minutes. Call task_report now.")

	assertToolUseAnswered(t, p.messages)

	if len(p.pendingResults) != 0 {
		t.Fatalf("expected pending results to be flushed, got %d", len(p.pendingResults))
	}
}

// assertToolUseAnswered enforces the Anthropic invariant: every tool_use block
// must have its matching tool_result in the immediately following message.
func assertToolUseAnswered(t *testing.T, messages []anthropic.MessageParam) {
	t.Helper()
	for i, m := range messages {
		var toolUseIDs []string
		for _, blk := range m.Content {
			if blk.OfToolUse != nil {
				toolUseIDs = append(toolUseIDs, blk.OfToolUse.ID)
			}
		}
		if len(toolUseIDs) == 0 {
			continue
		}
		answered := map[string]bool{}
		if i+1 < len(messages) {
			for _, blk := range messages[i+1].Content {
				if blk.OfToolResult != nil {
					answered[blk.OfToolResult.ToolUseID] = true
				}
			}
		}
		for _, id := range toolUseIDs {
			if !answered[id] {
				t.Fatalf("messages.%d tool_use %q has no tool_result in the next message", i, id)
			}
		}
	}
}

func TestAnthropicProviderPrune(t *testing.T) {
	model := "claude-opus-4-6"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)
	p.setInput("test")

	for range 30 {
		p.appendAssistant("response")
		p.appendUser("next")
	}

	dropped := p.prune(10)
	if dropped == 0 {
		t.Fatalf("expected some items to be pruned")
	}

	if got := p.userInput(); got != "test" {
		t.Fatalf("expected first message preserved after prune, got %q", got)
	}
}

func TestAnthropicProviderFullInput(t *testing.T) {
	model := "claude-opus-4-6"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)
	p.setInput("first")
	p.appendAssistant("response")
	p.appendUser("second")

	got := p.fullInput()
	if got == "" {
		t.Fatalf("expected non-empty full input")
	}
	if got != "first\n\nsecond" {
		t.Fatalf("expected 'first\\n\\nsecond', got %q", got)
	}
}

func TestAnthropicProviderSetReasoningEffort(t *testing.T) {
	model := "claude-opus-4-6"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)

	if p.params.Thinking.OfAdaptive != nil {
		t.Fatalf("expected no thinking before setReasoningEffort")
	}

	p.setReasoningEffort("high")
	if p.params.Thinking.OfAdaptive == nil {
		t.Fatalf("expected adaptive thinking after setting high")
	}
	if got := p.params.OutputConfig.Effort; got != anthropic.OutputConfigEffortHigh {
		t.Fatalf("expected effort high, got %q", got)
	}

	p.setReasoningEffort("medium")
	if got := p.params.OutputConfig.Effort; got != anthropic.OutputConfigEffortMedium {
		t.Fatalf("expected effort medium, got %q", got)
	}

	p.setReasoningEffort("")
	if got := p.params.OutputConfig.Effort; got != anthropic.OutputConfigEffortMedium {
		t.Fatalf("expected effort unchanged after empty, got %q", got)
	}
}

func TestAnthropicProviderOpus47Reasoning(t *testing.T) {
	model := "claude-opus-4-7"
	client := &Client{model: &model}
	effort := "high"
	client.reasoningEffort = &effort
	p := newAnthropicProvider(client, "instructions", nil)

	if p.params.Thinking.OfAdaptive == nil {
		t.Fatalf("expected adaptive thinking for opus-4-7 high")
	}
	if p.params.Thinking.OfEnabled != nil {
		t.Fatalf("expected no fixed-budget thinking for adaptive model")
	}
	if got := p.params.OutputConfig.Effort; got != anthropic.OutputConfigEffortHigh {
		t.Fatalf("expected effort high, got %q", got)
	}
	if p.params.MaxTokens != adaptiveThinkingMaxTokens {
		t.Fatalf("expected max tokens %d for adaptive high, got %d", adaptiveThinkingMaxTokens, p.params.MaxTokens)
	}
}

func TestAnthropicProviderLegacyReasoning(t *testing.T) {
	model := "claude-opus-4-5"
	client := &Client{model: &model}
	effort := "high"
	client.reasoningEffort = &effort
	p := newAnthropicProvider(client, "instructions", nil)

	if p.params.Thinking.OfAdaptive != nil {
		t.Fatalf("expected no adaptive thinking for legacy model")
	}
	if p.params.Thinking.OfEnabled == nil {
		t.Fatalf("expected fixed-budget thinking for legacy model")
	}
	if got := p.params.Thinking.OfEnabled.BudgetTokens; got != 16384 {
		t.Fatalf("expected budget 16384 for high, got %d", got)
	}
}

func TestAnthropicOutputEffort(t *testing.T) {
	tests := []struct {
		model  string
		effort string
		want   anthropic.OutputConfigEffort
	}{
		{"claude-opus-5", "none", anthropic.OutputConfigEffortLow},
		{"claude-opus-5", "minimal", anthropic.OutputConfigEffortLow},
		{"claude-opus-5", "low", anthropic.OutputConfigEffortLow},
		{"claude-opus-5", "medium", anthropic.OutputConfigEffortMedium},
		{"claude-opus-5", "high", anthropic.OutputConfigEffortHigh},
		{"claude-opus-5", "xhigh", anthropicEffortXHigh},
		{"claude-opus-4-8", "xhigh", anthropicEffortXHigh},
		{"claude-opus-4-7", "xhigh", anthropicEffortXHigh},
		// xhigh predates these; they only accept low/medium/high/max.
		{"claude-opus-4-6", "xhigh", anthropic.OutputConfigEffortMax},
		{"claude-sonnet-4-6", "xhigh", anthropic.OutputConfigEffortMax},
		// max is accepted by every adaptive model.
		{"claude-opus-5", "max", anthropic.OutputConfigEffortMax},
		{"claude-sonnet-4-6", "max", anthropic.OutputConfigEffortMax},
	}

	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.effort, func(t *testing.T) {
			got := anthropicOutputEffort(tt.model, tt.effort)
			if got != tt.want {
				t.Fatalf("anthropicOutputEffort(%q, %q) = %q, want %q", tt.model, tt.effort, got, tt.want)
			}
		})
	}
}

func TestAnthropicProviderOpus5Reasoning(t *testing.T) {
	model := "claude-opus-5"
	client := &Client{model: &model}
	effort := "xhigh"
	client.reasoningEffort = &effort
	p := newAnthropicProvider(client, "instructions", nil)

	if p.params.Thinking.OfAdaptive == nil {
		t.Fatalf("expected adaptive thinking for opus-5 xhigh")
	}
	if got := p.params.OutputConfig.Effort; got != anthropicEffortXHigh {
		t.Fatalf("expected effort xhigh, got %q", got)
	}
	if p.params.MaxTokens != adaptiveThinkingMaxTokens {
		t.Fatalf("expected max tokens %d for adaptive xhigh, got %d", adaptiveThinkingMaxTokens, p.params.MaxTokens)
	}
}

// On claude-opus-5 an omitted thinking field means adaptive thinking, so "none"
// has to send an explicit disabled block to actually turn thinking off.
func TestAnthropicProviderEffortNoneDisablesThinking(t *testing.T) {
	for _, model := range []string{"claude-opus-5", "claude-opus-4-8", "claude-sonnet-4-6"} {
		t.Run(model, func(t *testing.T) {
			m := model
			client := &Client{model: &m}
			effort := "none"
			client.reasoningEffort = &effort
			p := newAnthropicProvider(client, "instructions", nil)

			if p.params.Thinking.OfDisabled == nil {
				t.Fatalf("expected explicit disabled thinking for effort none")
			}
			if p.params.Thinking.OfAdaptive != nil {
				t.Fatalf("expected no adaptive thinking for effort none")
			}
			// Disabled thinking is only accepted at effort high or below.
			if got := p.params.OutputConfig.Effort; got != anthropic.OutputConfigEffortLow {
				t.Fatalf("expected effort low alongside disabled thinking, got %q", got)
			}
		})
	}
}

func TestAnthropicProviderReset(t *testing.T) {
	model := "claude-opus-4-6"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)

	p.setInput("timeline")
	p.appendUser("follow-up")

	if len(p.messages) < 2 {
		t.Fatalf("expected messages to have content before reset, got %d", len(p.messages))
	}

	p.reset()

	if len(p.messages) != 0 {
		t.Fatalf("expected empty messages after reset, got %d", len(p.messages))
	}
	if p.lastResponse != nil {
		t.Fatal("expected nil lastResponse after reset")
	}
	if len(p.pendingResults) != 0 {
		t.Fatalf("expected empty pendingResults after reset, got %d", len(p.pendingResults))
	}

	if len(p.params.System) == 0 || p.params.System[0].Text != "instructions" {
		t.Fatal("expected params (system instructions) to be preserved after reset")
	}
}

func TestBuildAnthropicTools(t *testing.T) {
	t.Run("string required", func(t *testing.T) {
		tools := buildAnthropicTools([]ToolDef{
			{
				Name:        "test_tool",
				Description: "a test tool",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "the query",
						},
					},
					"required":             []string{"query"},
					"additionalProperties": false,
				},
			},
		})

		if len(tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(tools))
		}
		if tools[0].OfTool == nil {
			t.Fatalf("expected OfTool to be set")
		}
		if tools[0].OfTool.Name != "test_tool" {
			t.Fatalf("expected name 'test_tool', got %q", tools[0].OfTool.Name)
		}
		if len(tools[0].OfTool.InputSchema.Required) != 1 || tools[0].OfTool.InputSchema.Required[0] != "query" {
			t.Fatalf("expected required=['query'], got %v", tools[0].OfTool.InputSchema.Required)
		}
	})

	t.Run("any required", func(t *testing.T) {
		tools := buildAnthropicTools([]ToolDef{
			{
				Name:        "test_tool",
				Description: "test",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
					"required":   []any{"a", "b"},
				},
			},
		})

		if len(tools[0].OfTool.InputSchema.Required) != 2 {
			t.Fatalf("expected 2 required fields, got %d", len(tools[0].OfTool.InputSchema.Required))
		}
	})
}

func TestThinkingBudget(t *testing.T) {
	tests := []struct {
		effort string
		want   int64
	}{
		{"low", 4096},
		{"minimal", 4096},
		{"medium", 8192},
		{"high", 16384},
		{"xhigh", 32768},
		{"", 0},
		{"none", 0},
	}

	for _, tt := range tests {
		t.Run(tt.effort, func(t *testing.T) {
			got := thinkingBudget(tt.effort)
			if got != tt.want {
				t.Fatalf("thinkingBudget(%q) = %d, want %d", tt.effort, got, tt.want)
			}
		})
	}
}

func TestIsAnthropicModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-opus-4-6", true},
		{"claude-sonnet-4-6", true},
		{"claude-haiku-4-5", true},
		{"gpt-5.4", false},
		{"gpt-4o", false},
		{"o3", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := isAnthropicModel(tt.model)
			if got != tt.want {
				t.Fatalf("isAnthropicModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestExtractAnthropicText(t *testing.T) {
	model := "claude-opus-4-6"
	client := &Client{model: &model}
	p := newAnthropicProvider(client, "instructions", nil)
	p.setInput("hello world")

	if got := extractAnthropicText(p.messages[0]); got != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got)
	}
}
