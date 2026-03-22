package llm

import (
	"testing"

	"github.com/openai/openai-go/v3/responses"
)

func TestActivationSetInput(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	s := NewActivation(client, NoopRecorder{}, "instructions", nil)

	s.SetInput("hello")
	if len(s.items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(s.items))
	}
	if got := extractInputFromItem(s.items[0]); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}

	s.SetInput("updated")
	if len(s.items) != 1 {
		t.Fatalf("expected 1 item after replace, got %d", len(s.items))
	}
	if got := extractInputFromItem(s.items[0]); got != "updated" {
		t.Fatalf("expected 'updated', got %q", got)
	}
}

func TestActivationAddToolResult(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	s := NewActivation(client, NoopRecorder{}, "instructions", nil)
	s.SetInput("test")

	call := ToolCall{CallID: "c1", Name: "db_query", Arguments: `{"query":"SELECT 1"}`}
	s.AddToolResult(call, `{"rows":[]}`, false)

	if len(s.items) != 3 {
		t.Fatalf("expected 3 items (input + call + output), got %d", len(s.items))
	}
	if len(s.history) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(s.history))
	}
	if s.history[0].Name != "db_query" {
		t.Fatalf("expected db_query in history, got %s", s.history[0].Name)
	}
}

func TestActivationPrune(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	s := NewActivation(client, NoopRecorder{}, "instructions", nil)
	s.SetInput("test")

	for i := range 50 {
		call := ToolCall{CallID: "c" + string(rune('0'+i%10)) + string(rune('0'+i/10)), Name: "db_query", Arguments: `{}`}
		s.items = append(s.items,
			responses.ResponseInputItemParamOfFunctionCall(`{}`, call.CallID, call.Name),
			responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, "result"),
		)
	}

	before := len(s.items)
	s.Prune()

	if len(s.items) >= before {
		t.Fatalf("expected items to be pruned, before=%d after=%d", before, len(s.items))
	}
	if s.items[0].OfMessage == nil {
		t.Fatalf("expected first item to still be user message")
	}
}

func TestActivationUsage(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	s := NewActivation(client, NoopRecorder{}, "instructions", nil)

	u := s.Usage()
	if u.TotalTokens != 0 {
		t.Fatalf("expected zero usage on new activation, got %d", u.TotalTokens)
	}
}

func TestActivationAppendMessages(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	s := NewActivation(client, NoopRecorder{}, "instructions", nil)
	s.SetInput("test")

	s.AppendAssistantText("thinking")
	s.AppendUserMessage("nudge")

	if len(s.items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(s.items))
	}

	s.AppendAssistantText("")
	if len(s.items) != 3 {
		t.Fatalf("empty text should not append, got %d items", len(s.items))
	}
}

func TestActivationRepeats(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	s := NewActivation(client, NoopRecorder{}, "instructions", nil)

	if s.Repeats() != 0 {
		t.Fatalf("expected 0 repeats on new activation, got %d", s.Repeats())
	}

	calls := []ToolCall{{CallID: "c1", Name: "db_query", Arguments: `{"q":"1"}`}}
	sig := roundSignature(calls)

	s.prevSig = sig
	s.repeats = 3

	if s.Repeats() != 3 {
		t.Fatalf("expected 3 repeats, got %d", s.Repeats())
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
