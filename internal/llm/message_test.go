package llm

import "testing"

func TestMarshalMessages(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		got := MarshalMessages(nil)
		if got != "[]" {
			t.Fatalf("expected %q, got %q", "[]", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := MarshalMessages([]Message{})
		if got != "[]" {
			t.Fatalf("expected %q, got %q", "[]", got)
		}
	})

	t.Run("with messages", func(t *testing.T) {
		msgs := []Message{
			{Role: "user", Content: "hello"},
			{Role: "tool_call", Content: "{}", Name: "done", CallID: "c1"},
		}
		got := MarshalMessages(msgs)
		if got == "[]" {
			t.Fatal("expected non-empty")
		}
	})
}

func TestUnmarshalMessages(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		msgs, err := UnmarshalMessages("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msgs != nil {
			t.Fatalf("expected nil, got %v", msgs)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		msgs, err := UnmarshalMessages("[]")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msgs != nil {
			t.Fatalf("expected nil, got %v", msgs)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		orig := []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "world"},
			{Role: "tool_call", Content: `{"q":"1"}`, Name: "db_query", CallID: "c1"},
			{Role: "tool_result", Content: "ok", CallID: "c1"},
		}
		s := MarshalMessages(orig)
		got, err := UnmarshalMessages(s)
		if err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(got) != len(orig) {
			t.Fatalf("expected %d messages, got %d", len(orig), len(got))
		}
		for i := range orig {
			if got[i].Role != orig[i].Role || got[i].Content != orig[i].Content {
				t.Fatalf("message %d mismatch: %+v vs %+v", i, orig[i], got[i])
			}
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := UnmarshalMessages("not json")
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}
