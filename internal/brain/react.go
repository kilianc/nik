package brain

import (
	"context"
	"log/slog"
	"time"
)

const reactionMinGap = 800 * time.Millisecond

type ToolReactor func(ctx context.Context, messageID, emoji string) error

type reactionQueueKey struct{}

type reactionQueue struct {
	ch   chan string
	done chan struct{}
}

func startReactionQueue(ctx context.Context, messageID string, reactor ToolReactor) *reactionQueue {
	q := &reactionQueue{
		ch:   make(chan string, 16),
		done: make(chan struct{}),
	}

	go q.drain(ctx, messageID, reactor)
	return q
}

func (q *reactionQueue) enqueue(emoji string) {
	select {
	case q.ch <- emoji:
	default:
	}
}

func (q *reactionQueue) close() {
	close(q.ch)
	<-q.done
}

func (q *reactionQueue) drain(ctx context.Context, messageID string, reactor ToolReactor) {
	defer close(q.done)

	var last time.Time
	sent := false

	for emoji := range q.ch {
		if gap := time.Since(last); gap < reactionMinGap {
			time.Sleep(reactionMinGap - gap)
		}

		err := reactor(ctx, messageID, emoji)
		if err != nil {
			slog.Warn("tool reaction failed", "pkg", "brain", "message_id", messageID, "emoji", emoji, "error", err)
		}

		last = time.Now()
		sent = true
	}

	if sent {
		if gap := time.Since(last); gap < reactionMinGap {
			time.Sleep(reactionMinGap - gap)
		}

		err := reactor(ctx, messageID, "")
		if err != nil {
			slog.Warn("tool reaction clear failed", "pkg", "brain", "message_id", messageID, "error", err)
		}
	}
}

func reactionQueueFromContext(ctx context.Context) *reactionQueue {
	q, _ := ctx.Value(reactionQueueKey{}).(*reactionQueue)
	return q
}
