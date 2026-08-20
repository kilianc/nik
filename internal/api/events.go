package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Events replace polling. Today the TUI runs three tickers against the
// database from its own process — messages, daemon liveness, alarm and task
// counts — and a browser console cannot run any of them. One stream, published
// by the process that already knows, serves both.
//
// Server-sent events rather than a websocket: this is one direction, it
// reconnects on its own, and it survives being tunnelled later without a
// second framing layer inside the first.

// Event types. A client switches on these, so they are a contract.
const (
	// EventMessage carries one Message that has just landed.
	EventMessage = "message"
	// EventActivity carries what nik is visibly doing in a conversation.
	EventActivity = "activity"
	// EventWorkload carries the counts a header renders.
	EventWorkload = "workload"
	// EventResync tells a client its stream fell behind and what it holds may
	// have gaps — refetch rather than trust it. See Broker.Publish.
	EventResync = "resync"
)

type Event struct {
	Type string
	Data any
}

// ActivityEvent is what nik is doing in one conversation, right now.
type ActivityEvent struct {
	ConversationID string   `json:"conversation_id"`
	Activity       []string `json:"activity"`
}

// WorkloadEvent is the count of things nik is carrying.
type WorkloadEvent struct {
	Alarms int `json:"alarms"`
	Tasks  int `json:"tasks"`
}

// Workload reports the same counts on demand. The stream only carries
// changes, so a client that has just connected needs somewhere to start.
type Workload interface {
	Counts(ctx context.Context) (WorkloadEvent, error)
}

func (s *Server) SetWorkload(workload Workload) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.workload = workload
}

func (s *Server) handleWorkload(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	workload := s.workload
	s.mu.RUnlock()

	if workload == nil {
		writeError(w, http.StatusServiceUnavailable, "nik is still starting")
		return
	}

	counts, err := workload.Counts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, counts)
}

// ResyncEvent says the stream dropped something.
type ResyncEvent struct {
	Reason string `json:"reason"`
}

// subscriberBuffer is how far behind a client may fall before it is told to
// resync. Deep enough to absorb a burst — nik answering in several messages
// while a console renders — shallow enough that a wedged reader cannot make
// nikd hold unbounded memory in a cell with a 320 MB budget.
const subscriberBuffer = 64

// Broker fans events out to whoever is listening.
//
// A slow subscriber is never allowed to slow down the publisher: nik's brain
// must not block because a browser tab is frozen. Overflowing subscribers are
// told they fell behind rather than silently missing messages, which is the
// difference between a console that refetches and one that quietly shows a
// conversation with holes in it.
type Broker struct {
	mu      sync.Mutex
	next    int
	clients map[int]*subscriber
}

type subscriber struct {
	ch       chan Event
	overflow bool
}

func NewBroker() *Broker {
	return &Broker{clients: map[int]*subscriber{}}
}

// Subscribe returns a channel of events and a function that closes it. The
// unsubscribe must be called, or the broker keeps publishing into a channel
// nobody reads until it overflows forever.
func (b *Broker) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.next
	b.next++

	sub := &subscriber{ch: make(chan Event, subscriberBuffer)}
	b.clients[id] = sub

	return sub.ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		if _, ok := b.clients[id]; !ok {
			return
		}
		delete(b.clients, id)
		close(sub.ch)
	}
}

// Publish never blocks. A subscriber whose buffer is full is marked, and the
// next event it can accept is a resync rather than the one it missed.
func (b *Broker) Publish(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, sub := range b.clients {
		if sub.overflow {
			// Already behind: try to hand it the resync and nothing else, so
			// a client that recovers learns what happened first.
			select {
			case sub.ch <- Event{Type: EventResync, Data: ResyncEvent{Reason: "stream fell behind"}}:
				sub.overflow = false
			default:
			}

			continue
		}

		select {
		case sub.ch <- event:
		default:
			sub.overflow = true
			slog.Warn("event subscriber fell behind", "pkg", "api", "type", event.Type)
		}
	}
}

// Subscribers is how many streams are open, for health and for tests.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.clients)
}

// heartbeatInterval keeps a tunnelled stream from being reaped by something
// in the middle that sees a quiet connection as a dead one. A nik can easily
// be silent for an hour.
const heartbeatInterval = 25 * time.Second

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// ResponseController rather than a type assertion on http.Flusher: the
	// logging middleware wraps the writer, and an assertion sees the wrapper
	// instead of what it wraps. Controllers unwrap.
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	err := rc.Flush()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	events, unsubscribe := s.broker.Subscribe()
	defer unsubscribe()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case event, open := <-events:
			if !open {
				return
			}

			err := writeSSE(w, event)
			if err != nil {
				// The client hung up mid-write. Not worth a log line above
				// Debug: it is what closing a terminal looks like from here.
				slog.Debug("event stream write", "pkg", "api", "error", err)
				return
			}
			_ = rc.Flush()

		case <-heartbeat.C:
			_, err := fmt.Fprint(w, ": ping\n\n")
			if err != nil {
				return
			}
			_ = rc.Flush()
		}
	}
}

func writeSSE(w io.Writer, event Event) error {
	payload, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("encode %s event: %w", event.Type, err)
	}

	// SSE frames are line-oriented, and a newline inside data would end the
	// frame early. json.Marshal never emits a raw newline, so one line is
	// always enough — asserting that here rather than hand-rolling a splitter
	// that would only ever run on impossible input.
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload)

	return err
}
