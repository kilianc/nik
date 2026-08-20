package apisvc

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/db"
)

// runWatcher starts a watcher polling fast enough that a test does not spend
// its life waiting, and returns the broker's stream.
func runWatcher(t *testing.T, chat *Chat) (<-chan api.Event, *api.Broker) {
	t.Helper()

	broker := api.NewBroker()
	watcher := NewWatcher(chat.conn, broker, chat)
	watcher.interval = 10 * time.Millisecond

	events, unsubscribe := broker.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		watcher.Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
		unsubscribe()
	})

	return events, broker
}

func waitFor(t *testing.T, events <-chan api.Event, eventType string) api.Event {
	t.Helper()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type == eventType {
				return event
			}
		case <-deadline:
			t.Fatalf("no %s event before timeout", eventType)
			return api.Event{}
		}
	}
}

// The first poll establishes where the conversation is rather than replaying
// it. A client fetches history on connect, so a backlog on the stream would
// only be a duplicate of what it already has.
func TestWatcherDoesNotReplayHistory(t *testing.T) {
	chat, _ := newTestChat(t)
	ctx := context.Background()

	err := chat.Send(ctx, api.LocalConversationID, "said before anyone was listening")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	events, _ := runWatcher(t, chat)

	// Workload always publishes on the first poll, so waiting for it proves a
	// poll happened — and that no message event came with it.
	waitFor(t, events, api.EventWorkload)

	select {
	case event := <-events:
		if event.Type == api.EventMessage {
			t.Fatal("watcher replayed a message that predates the stream")
		}
	default:
	}
}

func TestWatcherPublishesNewMessages(t *testing.T) {
	chat, _ := newTestChat(t)
	events, _ := runWatcher(t, chat)

	waitFor(t, events, api.EventWorkload)

	err := chat.Send(context.Background(), api.LocalConversationID, "the tap is dripping again")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	event := waitFor(t, events, api.EventMessage)
	msg, ok := event.Data.(api.Message)
	if !ok {
		t.Fatalf("event data is %T, want api.Message", event.Data)
	}
	if msg.Body != "the tap is dripping again" {
		t.Fatalf("body = %q", msg.Body)
	}
}

// A message published twice is a message rendered twice.
func TestWatcherDoesNotRepeatMessages(t *testing.T) {
	chat, _ := newTestChat(t)
	events, _ := runWatcher(t, chat)

	waitFor(t, events, api.EventWorkload)

	err := chat.Send(context.Background(), api.LocalConversationID, "only once please")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitFor(t, events, api.EventMessage)

	// Several more polls go by with nothing new.
	time.Sleep(100 * time.Millisecond)

	for {
		select {
		case event := <-events:
			if event.Type == api.EventMessage {
				t.Fatal("watcher published the same message twice")
			}
		default:
			return
		}
	}
}

func TestWatcherPublishesActivity(t *testing.T) {
	chat, conn := newTestChat(t)
	events, _ := runWatcher(t, chat)

	waitFor(t, events, api.EventWorkload)

	err := db.ConversationActivityPush(context.Background(), conn, db.LocalConversationID, "typing")
	if err != nil {
		t.Fatalf("push activity: %v", err)
	}

	event := waitFor(t, events, api.EventActivity)
	activity, ok := event.Data.(api.ActivityEvent)
	if !ok {
		t.Fatalf("event data is %T, want api.ActivityEvent", event.Data)
	}
	if len(activity.Activity) != 1 || activity.Activity[0] != "typing" {
		t.Fatalf("activity = %v, want [typing]", activity.Activity)
	}
}

// A watcher that republished unchanged state every 500ms would make the
// stream useless noise and every client grow a deduplicator.
func TestWatcherOnlyPublishesChanges(t *testing.T) {
	chat, _ := newTestChat(t)
	events, _ := runWatcher(t, chat)

	waitFor(t, events, api.EventWorkload)

	time.Sleep(100 * time.Millisecond)

	for {
		select {
		case event := <-events:
			t.Fatalf("unchanged state published a %s event", event.Type)
		default:
			return
		}
	}
}
