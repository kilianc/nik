package workbench

import (
	"testing"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestMarshalToolCalls(t *testing.T) {
	t.Run("with calls", func(t *testing.T) {
		rounds := []llm.RoundResult{
			{ToolCalls: []llm.ToolCall{{Name: "message_send", Arguments: `{"body":"hi"}`}}},
			{ToolCalls: []llm.ToolCall{{Name: "done", Arguments: `{}`}}},
		}

		got := marshalToolCalls(rounds)
		want := `[{"name":"message_send","arguments":"{\"body\":\"hi\"}"},{"name":"done","arguments":"{}"}]`
		if got != want {
			t.Fatalf("expected %s, got %s", want, got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		got := marshalToolCalls(nil)
		if got != "null" {
			t.Fatalf("expected null, got %s", got)
		}
	})
}

func TestLastOutput(t *testing.T) {
	t.Run("with rounds", func(t *testing.T) {
		rounds := []llm.RoundResult{
			{Text: "first"},
			{Text: ""},
			{Text: "last"},
		}

		got := lastOutput(rounds)
		if got != "last" {
			t.Fatalf("expected %q, got %q", "last", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		got := lastOutput(nil)
		if got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})
}
