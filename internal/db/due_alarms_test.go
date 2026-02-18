package db

import (
	"context"
	"testing"
	"time"
)

func TestDueAlarmsReturnsOnlyActiveAndDue(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)

	dueAlarm, err := CreateAlarm(ctx, conn, "", "", "due", "", now.Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("create due alarm: %v", err)
	}

	cancelledAlarm, err := CreateAlarm(ctx, conn, "", "", "cancelled", "", now.Add(-30*time.Second))
	if err != nil {
		t.Fatalf("create cancelled alarm: %v", err)
	}

	err = AlarmCancel(ctx, conn, cancelledAlarm.ID)
	if err != nil {
		t.Fatalf("cancel alarm: %v", err)
	}

	_, err = CreateAlarm(ctx, conn, "", "", "future", "", now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("create future alarm: %v", err)
	}

	alarms, err := DueAlarms(ctx, conn, now)
	if err != nil {
		t.Fatalf("due alarms: %v", err)
	}

	if len(alarms) != 1 {
		t.Fatalf("expected 1 due alarm, got %d", len(alarms))
	}
	if alarms[0].ID != dueAlarm.ID {
		t.Fatalf("expected due alarm id %q, got %q", dueAlarm.ID, alarms[0].ID)
	}
}

func TestDueAlarmsStillReturnsClaimedAlarms(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)

	alarm, err := CreateAlarm(ctx, conn, "", "", "claimed", "", now.Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	err = AlarmClaim(ctx, conn, alarm.ID, now)
	if err != nil {
		t.Fatalf("claim alarm: %v", err)
	}

	alarms, err := DueAlarms(ctx, conn, now)
	if err != nil {
		t.Fatalf("due alarms: %v", err)
	}

	if len(alarms) != 1 {
		t.Fatalf("expected claimed alarm to still appear as due, got %d", len(alarms))
	}
}
