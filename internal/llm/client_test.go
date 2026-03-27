package llm

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
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
		{"anthropic 500", &anthropic.Error{StatusCode: 500}, true},
		{"anthropic 502", &anthropic.Error{StatusCode: 502}, true},
		{"anthropic 429", &anthropic.Error{StatusCode: 429}, true},
		{"anthropic 400 not transient", &anthropic.Error{StatusCode: 400}, false},
		{"wrapped anthropic 503", fmt.Errorf("complete: %w", &anthropic.Error{StatusCode: 503}), true},

		{"tls bad record MAC", fmt.Errorf("round 6: remote error: tls: bad record MAC"), true},
		{"connection reset", fmt.Errorf("read tcp [::1]:1234->[::1]:443: read: connection reset by peer"), true},
		{"broken pipe", fmt.Errorf("write tcp [::1]:1234->[::1]:443: write: broken pipe"), true},
		{"unexpected EOF", fmt.Errorf("unexpected EOF"), true},
		{"i/o timeout", fmt.Errorf("read tcp [::1]:1234: i/o timeout"), true},
		{"tls protocol shutdown", fmt.Errorf("tls: protocol is shutdown"), true},
		{"dns no such host", fmt.Errorf(`Post "https://api.openai.com/v1/responses": dial tcp: lookup api.openai.com: no such host`), true},
		{"wrapped tls error", fmt.Errorf("complete round 4: %w", fmt.Errorf("remote error: tls: bad record MAC")), true},
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

func TestNewClient(t *testing.T) {
	t.Run("anthropic key only", func(t *testing.T) {
		model := "claude-opus-4-6"
		c := NewClient(&model, WithAnthropicKey("sk-ant-test"))

		if c.anthropicClient == nil {
			t.Fatalf("expected anthropic client to be initialized")
		}
		if c.apiClient != nil {
			t.Fatalf("expected openai api client to be nil")
		}
		if !c.isAnthropic() {
			t.Fatalf("expected isAnthropic() to be true for claude model")
		}
	})

	t.Run("both keys", func(t *testing.T) {
		model := "gpt-5.4"
		c := NewClient(&model, WithAPIKey("sk-test"), WithAnthropicKey("sk-ant-test"))

		if c.apiClient == nil {
			t.Fatalf("expected openai client to be initialized")
		}
		if c.anthropicClient == nil {
			t.Fatalf("expected anthropic client to be initialized")
		}
		if c.isAnthropic() {
			t.Fatalf("expected isAnthropic() to be false for gpt model")
		}
	})
}
