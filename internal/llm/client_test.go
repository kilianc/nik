package llm

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestIsImageMime(t *testing.T) {
	if !isImageMime("image/png") {
		t.Fatalf("expected image/png to be recognized as image mime")
	}
	if isImageMime("audio/ogg") {
		t.Fatalf("expected audio/ogg to not be recognized as image mime")
	}
}

func TestRoundSignature(t *testing.T) {
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

	type toolResult struct {
		result  string
		elapsed time.Duration
		isErr   bool
	}

	results := make([]toolResult, len(calls))

	start := time.Now()

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
}

func TestParallelToolExecutionWithErrors(t *testing.T) {
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

	type toolResult struct {
		result  string
		elapsed time.Duration
		isErr   bool
	}

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

	if results[0].result != "ok" || results[0].isErr {
		t.Fatalf("call 0: expected success, got %q (err=%v)", results[0].result, results[0].isErr)
	}

	if !results[1].isErr {
		t.Fatalf("call 1: expected error flag")
	}
	if results[1].result != `{"error":"boom"}` {
		t.Fatalf("call 1: expected error json, got %q", results[1].result)
	}
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
