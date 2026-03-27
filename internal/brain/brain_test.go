package brain

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestNewInitializesInternalState(t *testing.T) {
	b := New(&config.Config{}, nil)
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
}

func TestDoneToolAutoRegistered(t *testing.T) {
	b := New(&config.Config{}, nil)

	if _, ok := b.toolExec[doneToolName]; !ok {
		t.Fatal("expected done tool to be auto-registered")
	}

	found := false
	for _, def := range b.toolDefs {
		if def.Name == doneToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected done tool def in toolDefs")
	}
}

func TestAwakeDrainsActivationsBeforeReturning(t *testing.T) {
	b := New(&config.Config{}, nil)

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

func TestThinkSkipsGetInputAfterDone(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			fmt.Fprint(w, `{"id":"r1","object":"response","created_at":0,"status":"completed","output":[{"type":"function_call","id":"fc1","call_id":"c1","name":"done","arguments":"{\"reason\":\"test\"}","status":"completed"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
			return
		}
		fmt.Fprint(w, `{"id":"r2","object":"response","created_at":0,"status":"completed","output":[{"type":"message","id":"m1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"trace","annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	os.MkdirAll(promptsDir, 0o755)
	os.WriteFile(filepath.Join(promptsDir, "nik-00-base.md"), []byte("test"), 0o644)
	for _, name := range []string{"nik-01-identity.md", "nik-02-conversation.md", "nik-03-skills.md", "nik-04-brain.md"} {
		os.WriteFile(filepath.Join(promptsDir, name), []byte(""), 0o644)
	}

	model := "test-model"
	client := llm.NewClient(&model, llm.WithAPIKey("test-key"), llm.WithBaseURL(srv.URL))

	cfg := &config.Config{Home: tmpDir}
	b := New(cfg, client)
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
