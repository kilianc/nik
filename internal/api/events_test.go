package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrokerFansOutToEverySubscriber(t *testing.T) {
	broker := NewBroker()

	a, closeA := broker.Subscribe()
	defer closeA()
	b, closeB := broker.Subscribe()
	defer closeB()

	broker.Publish(Event{Type: EventMessage, Data: Message{ID: "1"}})

	for name, ch := range map[string]<-chan Event{"a": a, "b": b} {
		select {
		case got := <-ch:
			if got.Type != EventMessage {
				t.Errorf("%s got %q, want %q", name, got.Type, EventMessage)
			}
		default:
			t.Errorf("%s got nothing", name)
		}
	}
}

// The publisher is nik's brain. A frozen browser tab must never be able to
// slow it down, so Publish drops rather than blocks.
func TestPublishNeverBlocksOnASlowSubscriber(t *testing.T) {
	broker := NewBroker()

	_, unsubscribe := broker.Subscribe()
	defer unsubscribe()

	// Nobody is reading. Well past the buffer, so this deadlocks if Publish
	// ever waits.
	for range subscriberBuffer * 4 {
		broker.Publish(Event{Type: EventMessage, Data: Message{ID: "x"}})
	}
}

// Falling behind has to be visible. A console that silently misses messages
// shows a conversation with holes in it and no reason to refetch.
func TestOverflowedSubscriberIsToldToResync(t *testing.T) {
	broker := NewBroker()

	events, unsubscribe := broker.Subscribe()
	defer unsubscribe()

	for range subscriberBuffer + 10 {
		broker.Publish(Event{Type: EventMessage, Data: Message{ID: "x"}})
	}

	// Drain what fitted.
	for range subscriberBuffer {
		<-events
	}

	// The next publish is the one that can hand over the resync.
	broker.Publish(Event{Type: EventMessage, Data: Message{ID: "y"}})

	select {
	case got := <-events:
		if got.Type != EventResync {
			t.Fatalf("got %q, want %q after overflow", got.Type, EventResync)
		}
	default:
		t.Fatal("no resync after overflow")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	broker := NewBroker()

	events, unsubscribe := broker.Subscribe()
	if broker.Subscribers() != 1 {
		t.Fatalf("subscribers = %d, want 1", broker.Subscribers())
	}

	unsubscribe()

	if broker.Subscribers() != 0 {
		t.Fatalf("subscribers = %d after unsubscribe, want 0", broker.Subscribers())
	}
	if _, open := <-events; open {
		t.Fatal("channel still open after unsubscribe")
	}

	// A second unsubscribe is a double-close if the broker is careless, and
	// defer plus an explicit call is an easy way to get there.
	unsubscribe()
}

func TestSSEFrameFormat(t *testing.T) {
	var buf strings.Builder

	err := writeSSE(&buf, Event{Type: EventWorkload, Data: WorkloadEvent{Alarms: 2, Tasks: 1}})
	if err != nil {
		t.Fatalf("writeSSE: %v", err)
	}

	got := buf.String()
	want := "event: workload\ndata: {\"alarms\":2,\"tasks\":1}\n\n"
	if got != want {
		t.Fatalf("frame = %q, want %q", got, want)
	}
}

// The logging middleware wraps the ResponseWriter, and a wrapper that does not
// expose what it wraps silently strips http.Flusher — which turns the event
// stream into a connection that buffers forever and delivers nothing. The
// first draft of the handler did exactly this, and every unit test passed —
// only an end-to-end read caught it.
func TestMiddlewareKeepsTheWriterFlushable(t *testing.T) {
	var wrapped http.ResponseWriter

	handler := logRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		wrapped = w
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/events", nil))

	err := http.NewResponseController(wrapped).Flush()
	if err != nil {
		t.Fatalf("wrapped writer cannot flush: %v — the event stream would never stream", err)
	}
}
