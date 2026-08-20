package apisvc

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"time"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/db"
)

// Watcher turns the database into events.
//
// It polls, and that is deliberate for now: what it replaces is the same
// polling done worse — three tickers in the TUI's process, none available to a
// browser, all of them a second reader on a file nikd owns. One poller in the
// process that owns the data is strictly better, and it puts the seam in the
// right place. When messaging and the brain grow real publish points, this
// loop is what they replace, and no client changes.
type Watcher struct {
	conn   *sql.DB
	broker *api.Broker
	chat   *Chat

	interval time.Duration

	// Last-seen state, so only changes are published. A stream that repeats
	// itself is a stream every client has to deduplicate.
	lastMessageID string
	lastActivity  []string
	lastAlarms    int
	lastTasks     int
	seeded        bool
}

// defaultInterval is fast enough that a typing indicator feels live and slow
// enough to be invisible next to everything else nikd does on a tick.
const defaultInterval = 500 * time.Millisecond

func NewWatcher(conn *sql.DB, broker *api.Broker, chat *Chat) *Watcher {
	return &Watcher{
		conn:     conn,
		broker:   broker,
		chat:     chat,
		interval: defaultInterval,
		// -1 so the first poll always publishes, even on a nik with no
		// alarms and no tasks: a client that connects to silence cannot tell
		// "nothing yet" from "nothing at all".
		lastAlarms: -1,
		lastTasks:  -1,
	}
}

// Run polls until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *Watcher) poll(ctx context.Context) {
	w.pollMessages(ctx)
	w.pollActivity(ctx)
	w.pollWorkload(ctx)
}

func (w *Watcher) pollMessages(ctx context.Context) {
	messages, err := w.chat.Messages(ctx, api.MessagesQuery{
		ConversationID: api.LocalConversationID,
		After:          w.lastMessageID,
		Limit:          50,
	})
	if err != nil {
		slog.Warn("watch messages", "pkg", "apisvc", "error", err)
		return
	}
	// The first poll establishes where the conversation is; publishing a
	// backlog to nobody would only mean the first client to connect misses it
	// anyway. Clients fetch history on connect and stream from there.
	//
	// This runs before the empty check, and that ordering is the whole point:
	// seeding only on a poll that found something means a nik with an empty
	// local conversation never seeds, and the very first message anyone sends
	// it is swallowed as backlog.
	if !w.seeded {
		w.seeded = true
		if len(messages) > 0 {
			w.lastMessageID = messages[len(messages)-1].ID
		}

		return
	}

	if len(messages) == 0 {
		return
	}

	newest := messages[len(messages)-1].ID

	for _, msg := range messages {
		w.broker.Publish(api.Event{Type: api.EventMessage, Data: msg})
	}
	w.lastMessageID = newest
}

func (w *Watcher) pollActivity(ctx context.Context) {
	conv, err := w.chat.Conversation(ctx, api.LocalConversationID)
	if err != nil {
		slog.Warn("watch activity", "pkg", "apisvc", "error", err)
		return
	}

	if slices.Equal(conv.Activity, w.lastActivity) {
		return
	}
	w.lastActivity = slices.Clone(conv.Activity)

	w.broker.Publish(api.Event{
		Type: api.EventActivity,
		Data: api.ActivityEvent{
			ConversationID: api.LocalConversationID,
			Activity:       conv.Activity,
		},
	})
}

func (w *Watcher) pollWorkload(ctx context.Context) {
	alarms, err := db.AlarmCountActive(ctx, w.conn)
	if err != nil {
		slog.Warn("watch alarms", "pkg", "apisvc", "error", err)
		return
	}

	tasks, err := db.TaskCountActive(ctx, w.conn)
	if err != nil {
		slog.Warn("watch tasks", "pkg", "apisvc", "error", err)
		return
	}

	if alarms == w.lastAlarms && tasks == w.lastTasks {
		return
	}
	w.lastAlarms, w.lastTasks = alarms, tasks

	w.broker.Publish(api.Event{
		Type: api.EventWorkload,
		Data: api.WorkloadEvent{Alarms: alarms, Tasks: tasks},
	})
}
