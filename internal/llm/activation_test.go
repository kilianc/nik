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

func TestActivationConversation(t *testing.T) {
	models := []string{"gpt-5.4", "claude-opus-4-6"}

	for _, m := range models {
		t.Run(m, func(t *testing.T) {
			model := m
			client := &Client{model: &model}
			s := NewActivation(client, NoopRecorder{}, "instructions", nil)

			s.SetInput("timeline")
			s.AppendAssistantText("thinking")
			s.AppendUserMessage("nudge")

			call := ToolCall{CallID: "c1", Name: "done", Arguments: `{}`}
			s.AddToolResult(call, "ok", false)

			msgs := s.prov.conversation()
			if len(msgs) < 3 {
				t.Fatalf("expected at least 3 messages, got %d", len(msgs))
			}
			if msgs[0].Role != "user" || msgs[0].Content != "timeline" {
				t.Fatalf("unexpected first message: %+v", msgs[0])
			}

			serialized := MarshalMessages(msgs)
			if serialized == "[]" {
				t.Fatal("expected non-empty serialized messages")
			}
		})
	}
}

func TestActivationLoadHistory(t *testing.T) {
	models := []string{"gpt-5.4", "claude-opus-4-6"}

	for _, m := range models {
		t.Run(m, func(t *testing.T) {
			model := m
			client := &Client{model: &model}
			s := NewActivation(client, NoopRecorder{}, "instructions", nil)

			messages := []Message{
				{Role: "user", Content: "timeline"},
				{Role: "assistant", Content: "thinking"},
				{Role: "user", Content: "nudge"},
			}
			s.LoadHistory(messages)

			input := s.FullInput()
			if input == "" {
				t.Fatalf("expected non-empty full input after LoadHistory")
			}
		})
	}
}

func TestActivationResetConversation(t *testing.T) {
	models := []string{"gpt-5.4", "claude-opus-4-6"}

	for _, m := range models {
		t.Run(m, func(t *testing.T) {
			model := m
			client := &Client{model: &model}
			s := NewActivation(client, NoopRecorder{}, "instructions", nil)

			s.SetInput("timeline v0")
			call := ToolCall{CallID: "c1", Name: "test", Arguments: "{}"}
			s.AddToolResult(call, "ok", false)

			s.ResetConversation()

			if got := s.UserInput(); got != "" {
				t.Fatalf("expected empty user input after reset, got %q", got)
			}

			s.SetInput("timeline v1")

			if got := s.UserInput(); got != "timeline v1" {
				t.Fatalf("expected 'timeline v1' after re-set, got %q", got)
			}
		})
	}
}

func TestInjectReason(t *testing.T) {
	tools := []ToolDef{
		{
			Name:        "test_tool",
			Description: "a tool",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "some query",
					},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
	}

	result := injectReason(tools)

	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}

	props, _ := result[0].Parameters["properties"].(map[string]any)
	if _, ok := props["reason"]; !ok {
		t.Fatal("expected reason property to be injected")
	}

	req, _ := result[0].Parameters["required"].([]string)
	found := false
	for _, r := range req {
		if r == "reason" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected reason in required list")
	}

	if len(req) != 2 {
		t.Fatalf("expected 2 required fields, got %d", len(req))
	}

	origProps, _ := tools[0].Parameters["properties"].(map[string]any)
	if _, ok := origProps["reason"]; ok {
		t.Fatal("original tool should not be mutated")
	}

	origReq, _ := tools[0].Parameters["required"].([]string)
	if len(origReq) != 1 {
		t.Fatalf("original required should still have 1 entry, got %d", len(origReq))
	}
}

func TestInjectReasonEmptyProperties(t *testing.T) {
	tools := []ToolDef{
		{
			Name:        "no_params",
			Description: "empty tool",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"required":             []string{},
				"additionalProperties": false,
			},
		},
	}

	result := injectReason(tools)

	props, _ := result[0].Parameters["properties"].(map[string]any)
	if len(props) != 1 {
		t.Fatalf("expected 1 property (reason), got %d", len(props))
	}

	req, _ := result[0].Parameters["required"].([]string)
	if len(req) != 1 || req[0] != "reason" {
		t.Fatalf("expected [reason], got %v", req)
	}
}

func TestActivationState(t *testing.T) {
	model := "gpt-5.4"
	client := &Client{model: &model}

	t.Run("usage zero on new", func(t *testing.T) {
		s := NewActivation(client, NoopRecorder{}, "instructions", nil)
		u := s.Usage()
		if u.TotalTokens != 0 {
			t.Fatalf("expected zero usage on new activation, got %d", u.TotalTokens)
		}
	})

	t.Run("set max rounds", func(t *testing.T) {
		s := NewActivation(client, NoopRecorder{}, "instructions", nil)
		if s.maxRounds != 0 {
			t.Fatalf("expected 0 (use default), got %d", s.maxRounds)
		}

		s.SetMaxRounds(200)
		if s.maxRounds != 200 {
			t.Fatalf("expected 200, got %d", s.maxRounds)
		}
	})

	t.Run("repeats", func(t *testing.T) {
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
	})
}
