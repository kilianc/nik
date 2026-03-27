package llm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/responses"
)

func TestOpenAIProviderSetInput(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	p := newOpenAIProvider(client, "instructions", nil)

	if got := p.userInput(); got != "" {
		t.Fatalf("expected empty user input before setInput, got %q", got)
	}

	p.setInput("hello")
	if len(p.items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(p.items))
	}
	if got := extractInputFromItem(p.items[0]); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}

	p.setInput("updated")
	if len(p.items) != 1 {
		t.Fatalf("expected 1 item after replace, got %d", len(p.items))
	}
	if got := extractInputFromItem(p.items[0]); got != "updated" {
		t.Fatalf("expected 'updated', got %q", got)
	}
}

func TestOpenAIProviderAddToolResult(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	p := newOpenAIProvider(client, "instructions", nil)
	p.setInput("test")

	call := ToolCall{CallID: "c1", Name: "db_query", Arguments: `{"query":"SELECT 1"}`}
	p.addToolResult(call, `{"rows":[]}`, false)

	if len(p.items) != 3 {
		t.Fatalf("expected 3 items (input + call + output), got %d", len(p.items))
	}
}

func TestOpenAIProviderAppendMessages(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	p := newOpenAIProvider(client, "instructions", nil)
	p.setInput("test")

	p.appendAssistant("thinking")
	p.appendUser("nudge")

	if len(p.items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(p.items))
	}
}

func TestExtractInputFromItem(t *testing.T) {
	msg := responses.ResponseInputItemParamOfMessage("hello", responses.EasyInputMessageRoleUser)
	if got := extractInputFromItem(msg); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}

	fc := responses.ResponseInputItemParamOfFunctionCall(`{}`, "c1", "tool")
	if got := extractInputFromItem(fc); got != "" {
		t.Fatalf("expected empty for function call, got %q", got)
	}
}

func TestPruneItems(t *testing.T) {
	msg := responses.ResponseInputItemParamOfMessage("hello", responses.EasyInputMessageRoleUser)

	makePair := func(id string) (responses.ResponseInputItemUnionParam, responses.ResponseInputItemUnionParam) {
		fc := responses.ResponseInputItemParamOfFunctionCall(`{}`, id, "tool_"+id)
		fco := responses.ResponseInputItemParamOfFunctionCallOutput(id, "result_"+id)
		return fc, fco
	}

	buildItems := func(n int) responses.ResponseInputParam {
		items := responses.ResponseInputParam{msg}
		for i := range n {
			fc, fco := makePair(fmt.Sprintf("call_%d", i))
			items = append(items, fc, fco)
		}
		return items
	}

	t.Run("no-op under limit", func(t *testing.T) {
		items := buildItems(15)
		got := pruneItems(items, 20)
		if len(got) != len(items) {
			t.Fatalf("expected %d items, got %d", len(items), len(got))
		}
	})

	t.Run("no-op at exact limit", func(t *testing.T) {
		items := buildItems(20)
		got := pruneItems(items, 20)
		if len(got) != len(items) {
			t.Fatalf("expected %d items, got %d", len(items), len(got))
		}
	})

	t.Run("prunes one pair over limit", func(t *testing.T) {
		items := buildItems(21)
		got := pruneItems(items, 20)

		wantLen := 1 + 20*2
		if len(got) != wantLen {
			t.Fatalf("expected %d items, got %d", wantLen, len(got))
		}

		if got[0].OfMessage == nil {
			t.Fatalf("expected first item to be user message")
		}

		if got[1].OfFunctionCall == nil || got[1].OfFunctionCall.Name != "tool_call_1" {
			t.Fatalf("expected oldest kept pair to be call_1, got %+v", got[1].OfFunctionCall)
		}
	})

	t.Run("prunes many pairs", func(t *testing.T) {
		items := buildItems(50)
		got := pruneItems(items, 20)

		wantLen := 1 + 20*2
		if len(got) != wantLen {
			t.Fatalf("expected %d items, got %d", wantLen, len(got))
		}

		if got[1].OfFunctionCall == nil || got[1].OfFunctionCall.Name != "tool_call_30" {
			t.Fatalf("expected oldest kept pair to be call_30, got %+v", got[1].OfFunctionCall)
		}

		last := got[len(got)-1]
		if last.OfFunctionCallOutput == nil || last.OfFunctionCallOutput.CallID != "call_49" {
			t.Fatalf("expected last item to be output for call_49, got callID %q", last.OfFunctionCallOutput.CallID)
		}
	})

	t.Run("zero pairs", func(t *testing.T) {
		items := buildItems(0)
		got := pruneItems(items, 20)
		if len(got) != 1 {
			t.Fatalf("expected 1 item (user msg only), got %d", len(got))
		}
	})

	t.Run("one pair", func(t *testing.T) {
		items := buildItems(1)
		got := pruneItems(items, 20)
		if len(got) != 3 {
			t.Fatalf("expected 3 items, got %d", len(got))
		}
	})

	makeAssistant := func(text string) responses.ResponseInputItemUnionParam {
		return responses.ResponseInputItemParamOfMessage(text, responses.EasyInputMessageRoleAssistant)
	}

	buildMixedItems := func(rounds int, callsPerRound int) responses.ResponseInputParam {
		items := responses.ResponseInputParam{msg}
		for r := range rounds {
			items = append(items, makeAssistant(fmt.Sprintf("round_%d_thinking", r)))
			for c := range callsPerRound {
				id := fmt.Sprintf("r%d_c%d", r, c)
				fc, fco := makePair(id)
				items = append(items, fc, fco)
			}
		}
		return items
	}

	t.Run("mixed no-op under limit", func(t *testing.T) {
		items := buildMixedItems(3, 2)
		got := pruneItems(items, 20)
		if len(got) != len(items) {
			t.Fatalf("expected %d items, got %d", len(items), len(got))
		}
	})

	t.Run("mixed prunes old rounds with their assistant messages", func(t *testing.T) {
		items := buildMixedItems(5, 5)
		got := pruneItems(items, 20)

		var keptPairs int
		for _, item := range got[1:] {
			if item.OfFunctionCallOutput != nil {
				keptPairs++
			}
		}
		if keptPairs != 20 {
			t.Fatalf("expected 20 kept pairs, got %d", keptPairs)
		}

		if got[0].OfMessage == nil {
			t.Fatalf("expected first item to be user message")
		}
	})

	t.Run("mixed keeps assistant messages for surviving rounds", func(t *testing.T) {
		items := buildMixedItems(5, 5)
		got := pruneItems(items, 20)

		var assistantCount int
		for _, item := range got[1:] {
			if item.OfMessage != nil {
				assistantCount++
			}
		}

		if assistantCount == 0 {
			t.Fatal("expected at least one assistant message to survive pruning")
		}
	})
}

func TestBuildToolParamsIncludesDefinitions(t *testing.T) {
	params := buildToolParams([]ToolDef{
		{
			Name:        "test_tool",
			Description: "test",
			Parameters: map[string]any{
				"type": "object",
			},
		},
	})

	if len(params) != 1 {
		t.Fatalf("expected 1 tool param, got %d", len(params))
	}
	if params[0].OfFunction == nil || params[0].OfFunction.Name != "test_tool" {
		t.Fatalf("expected function tool named test_tool, got %+v", params[0])
	}
}

func TestEnsureJSONInput(t *testing.T) {
	if got := ensureJSONInput("", false); got != "" {
		t.Fatalf("expected empty input without json mode, got %q", got)
	}

	if got := ensureJSONInput("already json", true); got != "already json" {
		t.Fatalf("expected non-empty input to pass through, got %q", got)
	}

	if got := ensureJSONInput("   ", true); got != jsonObjectInputHint {
		t.Fatalf("expected json hint for blank input, got %q", got)
	}

	if !strings.Contains(strings.ToLower(jsonObjectInputHint), "json") {
		t.Fatalf("expected json hint to mention json, got %q", jsonObjectInputHint)
	}
}

func TestMaxPairsForModel(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-5.4", maxHistoryPairs},
		{"gpt-4.1", maxHistoryPairs},
		{"gpt-5.4-mini", 50},
		{"gpt-5.3-codex", 50},
		{"gpt-4o", 16},
		{"o1-mini", 16},
		{"o3", 25},
		{"unknown-model", maxHistoryPairs},
		{"claude-opus-4-6", 25},
		{"claude-sonnet-4-6", 25},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := maxPairsForModel(tt.model)
			if got != tt.want {
				ctx, _ := ModelContextWindow(tt.model)
				t.Fatalf("maxPairsForModel(%q) = %d, want %d (context_window=%d)", tt.model, got, tt.want, ctx)
			}
		})
	}
}
