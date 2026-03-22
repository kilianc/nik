package brain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
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
	if len(b.toolDefs) != 0 {
		t.Fatalf("expected no tools on startup")
	}
	if b.sensor != nil {
		t.Fatalf("expected sensor to be nil on startup")
	}
}

func TestTerminalTools(t *testing.T) {
	terminal := []string{"message_send", "message_noop", "message_react"}
	for _, name := range terminal {
		if !terminalTools[name] {
			t.Errorf("expected %q to be terminal", name)
		}
	}

	nonTerminal := []string{"task_list", "db_query", "message_set_presence", "describe_media"}
	for _, name := range nonTerminal {
		if terminalTools[name] {
			t.Errorf("expected %q to NOT be terminal", name)
		}
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
