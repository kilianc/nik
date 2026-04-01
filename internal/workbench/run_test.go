package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kciuffolo/nik/internal/db"
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

	t.Run("empty", func(t *testing.T) {
		got := marshalToolCalls(nil)
		if got != "[]" {
			t.Fatalf("expected [], got %s", got)
		}
	})
}

func TestRunLoadsFullHistory(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "initial input"},
		{Role: "tool_call", Content: `{"reason":"test"}`, Name: "load_skill", CallID: "call_1"},
		{Role: "tool_result", Content: "skill loaded", CallID: "call_1"},
		{Role: "user", Content: "continue working"},
	}
	messagesJSON, _ := json.Marshal(messages)

	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, openaiTextResponse("done"))
	}))
	defer srv.Close()

	model := "test-model"
	run := db.ExperimentVariantRun{
		Model:        model,
		Instructions: "test instructions",
		ToolSchemas:  "[]",
		Messages:     string(messagesJSON),
	}

	opts := []llm.ClientOption{llm.WithAPIKey("test-key"), llm.WithBaseURL(srv.URL)}
	_, err := Run(context.Background(), run, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	input, ok := capturedBody["input"].([]any)
	if !ok {
		t.Fatalf("expected input to be array, got %T", capturedBody["input"])
	}

	if len(input) != len(messages) {
		t.Fatalf("expected %d input items (full history), got %d", len(messages), len(input))
	}
}

func openaiTextResponse(text string) string {
	output := []map[string]any{
		{"type": "message", "role": "assistant", "content": []map[string]any{
			{"type": "output_text", "text": text},
		}},
	}
	b, _ := json.Marshal(map[string]any{
		"id": "r1", "object": "response", "created_at": 0, "status": "completed",
		"output_text": text,
		"output":      output,
		"usage": map[string]any{
			"input_tokens": 10, "output_tokens": 5, "total_tokens": 15,
			"input_tokens_details":  map[string]any{"cached_tokens": 0},
			"output_tokens_details": map[string]any{"reasoning_tokens": 0},
		},
	})
	return string(b)
}
