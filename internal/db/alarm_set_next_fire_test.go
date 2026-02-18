package db

import (
	"context"
	"testing"
	"time"
)

func TestAlarmSetNextFireUpdatesNextFireAt(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, "", "", "recurring", "every day at 9am", now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	err = AlarmClaim(ctx, conn, alarm.ID, now)
	if err != nil {
		t.Fatalf("claim alarm: %v", err)
	}

	nextFire := now.Add(24 * time.Hour)
	err = AlarmSetNextFire(ctx, conn, alarm.ID, nextFire)
	if err != nil {
		t.Fatalf("set next fire: %v", err)
	}

	alarms, err := DueAlarms(ctx, conn, nextFire.Add(time.Second))
	if err != nil {
		t.Fatalf("due alarms: %v", err)
	}
	if len(alarms) != 1 {
		t.Fatalf("expected 1 due alarm, got %d", len(alarms))
	}
	if alarms[0].ID != alarm.ID {
		t.Fatalf("expected alarm id %q, got %q", alarm.ID, alarms[0].ID)
	}
}
