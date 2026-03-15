package skills

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestListEventsReturnsSinceTimestamp(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	before := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	_, err = db.SkillEventInsert(ctx, conn, db.SkillEventInsertParams{
		Name:        "journal",
		Kind:        "added",
		ContentHash: "abc",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	svc := NewService(conn)

	events, err := svc.ListEvents(ctx, before)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "journal" {
		t.Errorf("expected name 'journal', got %q", events[0].Name)
	}

	events, err = svc.ListEvents(ctx, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("list events future: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events for future since, got %d", len(events))
	}
}
