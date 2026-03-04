package brain

import (
	"context"
	"time"
)

const reactionMinGap = 800 * time.Millisecond

// ToolReactor sends a reaction emoji to the message that triggered an activation.
type ToolReactor func(ctx context.Context, messageID, emoji string)

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

		reactor(ctx, messageID, emoji)
		last = time.Now()
		sent = true
	}

	if sent {
		if gap := time.Since(last); gap < reactionMinGap {
			time.Sleep(reactionMinGap - gap)
		}
		reactor(ctx, messageID, "")
	}
}

func reactionQueueFromContext(ctx context.Context) *reactionQueue {
	q, _ := ctx.Value(reactionQueueKey{}).(*reactionQueue)
	return q
}
