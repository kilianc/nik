package llm

import (
	"testing"
)

func TestActivationSetInput(t *testing.T) {
	models := []string{"gpt-5.4", "claude-opus-4-6"}

	for _, m := range models {
		t.Run(m, func(t *testing.T) {
			model := m
			client := &Client{model: &model}
			s := NewActivation(client, NoopRecorder{}, "instructions", nil)

			s.SetInput("hello")
			if got := s.UserInput(); got != "hello" {
				t.Fatalf("expected 'hello', got %q", got)
			}

			s.SetInput("updated")
			if got := s.UserInput(); got != "updated" {
				t.Fatalf("expected 'updated', got %q", got)
			}
		})
	}
}

func TestActivationAddToolResult(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}
	s := NewActivation(client, NoopRecorder{}, "instructions", nil)
	s.SetInput("test")

	call := ToolCall{CallID: "c1", Name: "db_query", Arguments: `{"query":"SELECT 1"}`}
	s.AddToolResult(call, `{"rows":[]}`, false)

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
		s.AddToolResult(call, "result", false)
	}

	s.Prune()

	if got := s.UserInput(); got != "test" {
		t.Fatalf("expected first item to still be user message, got %q", got)
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

	input := s.FullInput()
	if input == "" {
		t.Fatalf("expected non-empty full input")
	}

	s.AppendAssistantText("")
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
