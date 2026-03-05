package db

import (
	"context"
	"testing"
	"time"
)

func TestAlarmOccurrenceInsertPersistsRow(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		Goal:       "test",
		NextFireAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	occ, err := AlarmOccurrenceInsert(ctx, conn, alarm.ID, now)
	if err != nil {
		t.Fatalf("insert occurrence: %v", err)
	}

	if occ.ID == "" {
		t.Fatalf("expected occurrence id")
	}
	if occ.AlarmID != alarm.ID {
		t.Fatalf("expected alarm_id %q, got %q", alarm.ID, occ.AlarmID)
	}
	if occ.Note.Valid {
		t.Fatalf("expected null note")
	}
}
