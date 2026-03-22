package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
)

func TestIsImageMime(t *testing.T) {
	if !isImageMime("image/png") {
		t.Fatalf("expected image/png to be recognized as image mime")
	}
	if isImageMime("audio/ogg") {
		t.Fatalf("expected audio/ogg to not be recognized as image mime")
	}
}

func Test_roundSignature(t *testing.T) {
	a := ToolCall{Name: "load_skill", Arguments: `{"action":"load","name":"search"}`}
	b := ToolCall{Name: "db_query", Arguments: `{"query":"SELECT 1"}`}

	sig1 := roundSignature([]ToolCall{a})
	sig2 := roundSignature([]ToolCall{a})
	if sig1 != sig2 {
		t.Fatalf("identical calls should produce identical signatures")
	}

	sig3 := roundSignature([]ToolCall{a, b})
	sig4 := roundSignature([]ToolCall{b, a})
	if sig3 != sig4 {
		t.Fatalf("order should not matter: %q != %q", sig3, sig4)
	}

	different := ToolCall{Name: "load_skill", Arguments: `{"action":"load","name":"alarm"}`}
	sig5 := roundSignature([]ToolCall{different})
	if sig1 == sig5 {
		t.Fatalf("different args should produce different signatures")
	}
}

func TestSpeechRequiresAPIKey(t *testing.T) {
	client := &Client{}

	_, err := client.Speech(t.Context(), "hello", "gpt-4o-mini-tts", "ash", "", 1.0)
	if err == nil {
		t.Fatalf("expected error when apiClient is nil")
	}
	if err.Error() != "speech: requires api key" {
		t.Fatalf("expected 'requires api key' error, got %v", err)
	}
}

func TestParallelToolExecution(t *testing.T) {
	type toolResult struct {
		result  string
		elapsed time.Duration
		isErr   bool
	}

	runParallel := func(executor ToolExecutor, calls []ToolCall) []toolResult {
		results := make([]toolResult, len(calls))
		var wg sync.WaitGroup
		wg.Add(len(calls))
		for i, call := range calls {
			go func(i int, call ToolCall) {
				defer wg.Done()
				s := time.Now()
				result, err := executor(context.Background(), call)
				elapsed := time.Since(s)
				if err != nil {
					results[i] = toolResult{result: ToolError(err), elapsed: elapsed, isErr: true}
					return
				}
				results[i] = toolResult{result: result, elapsed: elapsed}
			}(i, call)
		}
		wg.Wait()
		return results
	}

	t.Run("runs concurrently", func(t *testing.T) {
		const delay = 50 * time.Millisecond
		executor := func(_ context.Context, call ToolCall) (string, error) {
			time.Sleep(delay)
			return fmt.Sprintf("result-%s", call.CallID), nil
		}
		calls := []ToolCall{
			{CallID: "a", Name: "tool1", Arguments: `{}`},
			{CallID: "b", Name: "tool2", Arguments: `{}`},
			{CallID: "c", Name: "tool3", Arguments: `{}`},
		}

		start := time.Now()
		results := runParallel(executor, calls)
		total := time.Since(start)

		if total >= delay*time.Duration(len(calls)) {
			t.Fatalf("expected parallel execution (<%v), took %v", delay*time.Duration(len(calls)), total)
		}
		for i, call := range calls {
			expected := fmt.Sprintf("result-%s", call.CallID)
			if results[i].result != expected {
				t.Fatalf("call %d: expected %q, got %q", i, expected, results[i].result)
			}
			if results[i].isErr {
				t.Fatalf("call %d: unexpected error", i)
			}
		}
	})

	t.Run("captures errors", func(t *testing.T) {
		executor := func(_ context.Context, call ToolCall) (string, error) {
			if call.Name == "fail" {
				return "", fmt.Errorf("boom")
			}
			return "ok", nil
		}
		calls := []ToolCall{
			{CallID: "a", Name: "succeed", Arguments: `{}`},
			{CallID: "b", Name: "fail", Arguments: `{}`},
		}

		results := runParallel(executor, calls)

		if results[0].result != "ok" || results[0].isErr {
			t.Fatalf("call 0: expected success, got %q (err=%v)", results[0].result, results[0].isErr)
		}
		if !results[1].isErr {
			t.Fatalf("call 1: expected error flag")
		}
		if results[1].result != `{"error":"boom"}` {
			t.Fatalf("call 1: expected error json, got %q", results[1].result)
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

func TestMaxPairsForModel(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-5.4", maxHistoryPairs},
		{"gpt-4.1", maxHistoryPairs},
		{"gpt-5.4-mini", 25},
		{"gpt-5.3-codex", 25},
		{"gpt-4o", minHistoryPairs},
		{"o1-mini", minHistoryPairs},
		{"o3", 12},
		{"unknown-model", maxHistoryPairs},
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

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"plain error", fmt.Errorf("something broke"), false},
		{"openai 500", &openai.Error{StatusCode: 500}, true},
		{"openai 502", &openai.Error{StatusCode: 502}, true},
		{"openai 429 is not transient", &openai.Error{StatusCode: 429}, false},
		{"stream server_error", &ssestream.StreamError{Message: "server_error"}, true},
		{"stream INTERNAL_ERROR", &ssestream.StreamError{Message: "stream ID 1; INTERNAL_ERROR; received from peer"}, true},
		{"stream unrelated message", &ssestream.StreamError{Message: "connection reset"}, false},
		{"wrapped openai 503", fmt.Errorf("complete: %w", &openai.Error{StatusCode: 503}), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransient(tt.err)
			if got != tt.want {
				t.Errorf("IsTransient() = %v, want %v", got, tt.want)
			}
		})
	}
}
