package nikapi

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/api"
)

// serveWithBroker is the pair again: a real server on a real socket, so the
// SSE framing is proved by something that parses it rather than by a fixture.
func serveWithBroker(t *testing.T) (*Client, *api.Broker) {
	t.Helper()

	srv := api.New(api.NewState())
	client := serveServer(t, srv)

	return client, srv.Broker()
}

func TestEventsStreamMessages(t *testing.T) {
	client, broker := serveWithBroker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got := make(chan Event, 4)
	streamErr := make(chan error, 1)
	go func() {
		streamErr <- client.Events(ctx, func(e Event) { got <- e })
	}()

	// The subscription is established inside the handler, so publishing
	// immediately would race it. Wait for the server to see us.
	waitForSubscriber(t, broker)

	broker.Publish(api.Event{
		Type: api.EventMessage,
		Data: api.Message{ID: "m1", Kind: "text", Body: "dinner's ready"},
	})

	select {
	case event := <-got:
		if event.Type != EventMessage {
			t.Fatalf("type = %q, want %q", event.Type, EventMessage)
		}
		msg, err := event.Message()
		if err != nil {
			t.Fatalf("decode message: %v", err)
		}
		if msg.Body != "dinner's ready" {
			t.Fatalf("body = %q", msg.Body)
		}
	case err := <-streamErr:
		t.Fatalf("stream ended early: %v", err)
	case <-ctx.Done():
		t.Fatal("no event before timeout")
	}
}

func TestEventsStreamActivityAndWorkload(t *testing.T) {
	client, broker := serveWithBroker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got := make(chan Event, 8)
	go func() { _ = client.Events(ctx, func(e Event) { got <- e }) }()

	waitForSubscriber(t, broker)

	broker.Publish(api.Event{
		Type: api.EventActivity,
		Data: api.ActivityEvent{ConversationID: "local", Activity: []string{"typing"}},
	})
	broker.Publish(api.Event{
		Type: api.EventWorkload,
		Data: api.WorkloadEvent{Alarms: 3, Tasks: 1},
	})

	activity, err := next(t, ctx, got).Activity()
	if err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	if len(activity.Activity) != 1 || activity.Activity[0] != "typing" {
		t.Fatalf("activity = %v, want [typing]", activity.Activity)
	}

	workload, err := next(t, ctx, got).Workload()
	if err != nil {
		t.Fatalf("decode workload: %v", err)
	}
	if workload.Alarms != 3 || workload.Tasks != 1 {
		t.Fatalf("workload = %+v, want 3 alarms and 1 task", workload)
	}
}

// Cancelling has to end the stream and release the subscription, or a TUI
// that reconnects leaks one every time.
func TestEventsStopOnContextCancel(t *testing.T) {
	client, broker := serveWithBroker(t)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- client.Events(ctx, func(Event) {}) }()

	waitForSubscriber(t, broker)
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stream did not end after cancel")
	}

	deadline := time.Now().Add(5 * time.Second)
	for broker.Subscribers() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("subscribers = %d after cancel, want 0", broker.Subscribers())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEventsWithNoDaemonIsTyped(t *testing.T) {
	client := NewAtSocket(shortHome(t) + "/nothing.sock")

	err := client.Events(context.Background(), func(Event) {})
	if err != ErrNoDaemon {
		t.Fatalf("err = %v, want ErrNoDaemon", err)
	}
}

func next(t *testing.T, ctx context.Context, ch <-chan Event) Event {
	t.Helper()

	select {
	case e := <-ch:
		return e
	case <-ctx.Done():
		t.Fatal("no event before timeout")
		return Event{}
	}
}

func waitForSubscriber(t *testing.T, broker *api.Broker) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for broker.Subscribers() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("client never subscribed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
