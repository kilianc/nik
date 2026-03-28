package llm

import (
	"testing"
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

	if p.params.Thinking.OfEnabled != nil {
		t.Fatalf("expected no thinking before setReasoningEffort")
	}

	p.setReasoningEffort("high")
	if p.params.Thinking.OfEnabled == nil {
		t.Fatalf("expected thinking to be enabled after setting high")
	}
	if got := p.params.Thinking.OfEnabled.BudgetTokens; got != 16384 {
		t.Fatalf("expected budget 16384 for high, got %d", got)
	}

	p.setReasoningEffort("low")
	if got := p.params.Thinking.OfEnabled.BudgetTokens; got != 4096 {
		t.Fatalf("expected budget 4096 for low, got %d", got)
	}

	p.setReasoningEffort("")
	if got := p.params.Thinking.OfEnabled.BudgetTokens; got != 4096 {
		t.Fatalf("expected budget unchanged after empty, got %d", got)
	}

	p.setReasoningEffort("medium")
	if got := p.params.Thinking.OfEnabled.BudgetTokens; got != 8192 {
		t.Fatalf("expected budget 8192 for medium, got %d", got)
	}
	if p.params.MaxTokens < 8192+1024 {
		t.Fatalf("expected max tokens to be bumped for medium budget, got %d", p.params.MaxTokens)
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
