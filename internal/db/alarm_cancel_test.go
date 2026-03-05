package db

import (
	"context"
	"testing"
	"time"
)

func TestAlarmCancelRemovesFromDueList(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		Goal:       "cancel me",
		NextFireAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	before, err := DueAlarms(ctx, conn, now)
	if err != nil {
		t.Fatalf("due alarms before cancel: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("expected 1 due alarm, got %d", len(before))
	}

	err = AlarmCancel(ctx, conn, alarm.ID)
	if err != nil {
		t.Fatalf("cancel alarm: %v", err)
	}

	after, err := DueAlarms(ctx, conn, now)
	if err != nil {
		t.Fatalf("due alarms after cancel: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected no due alarms after cancel, got %d", len(after))
	}
}
