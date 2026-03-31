package brain

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/prompt"
)

func TestNewInitializesInternalState(t *testing.T) {
	b := New(&config.Config{}, nil, prompt.NewRenderer(&config.Config{Home: t.TempDir()}))
	if b == nil {
		t.Fatalf("expected non-nil brain")
	}
	if b.now == nil {
		t.Fatalf("expected now function to be initialized")
	}
	if b.toolExec == nil || b.privileged == nil {
		t.Fatalf("expected maps to be initialized")
	}
	if b.claimed == nil {
		t.Fatalf("expected sync set to be initialized")
	}
	if len(b.toolDefs) != 1 || b.toolDefs[0].Name != doneToolName {
		t.Fatalf("expected only done tool on startup, got %d tools", len(b.toolDefs))
	}
	if b.sensor != nil {
		t.Fatalf("expected sensor to be nil on startup")
	}

	if _, ok := b.toolExec[doneToolName]; !ok {
		t.Fatal("expected done tool executor to be auto-registered")
	}
}

func TestAwakeDrainsActivationsBeforeReturning(t *testing.T) {
	b := New(&config.Config{}, nil, prompt.NewRenderer(&config.Config{Home: t.TempDir()}))

	var activationDone atomic.Bool

	b.SetSensor(&fakeSensor{
		checkOnce: true,
		stimuli: []Stimulus{
			{Meta: map[string]string{"conversation_id": "test-conv", "sources": "[]"}},
		},
	})

	origActivate := b.activate
	_ = origActivate

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		time.Sleep(200 * time.Millisecond)
		activationDone.Store(true)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		b.Awake(ctx, 50*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		if !activationDone.Load() {
			t.Fatal("Awake returned before activation finished")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Awake did not return within timeout")
	}
}

type fakeSensor struct {
	checkOnce bool
	called    atomic.Bool
	stimuli   []Stimulus
}

func (f *fakeSensor) Check(_ context.Context) ([]Stimulus, error) {
	if f.checkOnce && f.called.Swap(true) {
		return nil, nil
	}
	return f.stimuli, nil
}

func (f *fakeSensor) Read(_ context.Context, _ string) string {
	return ""
}

func (f *fakeSensor) Peek(_ context.Context, _ string) string {
	return ""
}

func TestThinkExitsImmediatelyOnDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"r1","object":"response","created_at":0,"status":"completed","output":[{"type":"function_call","id":"fc1","call_id":"c1","name":"done","arguments":"{\"reason\":\"test\"}","status":"completed"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()

	model := "test-model"
	client := llm.NewClient(&model, llm.WithAPIKey("test-key"), llm.WithBaseURL(srv.URL))

	cfg := &config.Config{Home: tmpDir}
	b := New(cfg, client, prompt.NewRenderer(cfg))
	b.now = func() time.Time { return time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC) }

	var getInputCalls atomic.Int32
	getInput := func() string {
		getInputCalls.Add(1)
		return "test timeline"
	}

	ctx := context.WithValue(context.Background(), "meta", map[string]string{
		"conversation_id": "test-conv",
		"activation_id":   "test-act",
	})

	_, _, err := b.think(ctx, getInput)
	if err != nil {
		t.Fatalf("think: %v", err)
	}

	if got := getInputCalls.Load(); got != 1 {
		t.Fatalf("expected getInput called 1 time (initial read only), got %d", got)
	}
}
