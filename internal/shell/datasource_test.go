package shell

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/messaging"
)

func TestFormatConversationContextReturnsNilWithoutMessages(t *testing.T) {
	lines := formatConversationContext(nil, nil)
	if lines != nil {
		t.Fatalf("expected nil lines for empty message context, got %v", lines)
	}
}

func TestDeadSessionRetrigger(t *testing.T) {
	requireTmux(t)

	id := "test-retrigger"
	defer cleanup(t, id)

	err := newSession(id, "echo done", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	stare(id, 5)

	if isAlive(id) {
		t.Fatal("expected session to be dead")
	}

	now := time.Now().UTC()
	err = saveMeta(id, SessionMeta{
		Command:      "echo done",
		Description:  "test",
		ActivationID: "finished-run",
		NextCheckAt:  now.Add(5 * time.Minute),
		StartedAt:    now,
	})
	if err != nil {
		t.Fatalf("saveMeta: %v", err)
	}

	ds := NewDataSource(new(messaging.Service), func(string) bool { return false })

	outputs, err := ds.Check(context.Background())
	if err != nil {
		t.Fatalf("first Check: %v", err)
	}

	found := 0
	for _, o := range outputs {
		if o.Meta["source_id"] == id {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("first Check: expected 1 output for session %s, got %d", id, found)
	}

	// tool handler kills the session when model calls shell read on a dead pane
	killSession(id)

	outputs2, err := ds.Check(context.Background())
	if err != nil {
		t.Fatalf("second Check: %v", err)
	}

	found = 0
	for _, o := range outputs2 {
		if o.Meta["source_id"] == id {
			found++
		}
	}
	if found != 0 {
		t.Fatalf("second Check: expected 0 outputs for session %s (should be cleaned up), got %d", id, found)
	}
}
