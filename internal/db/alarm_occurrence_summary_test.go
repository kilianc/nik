package db

import (
	"context"
	"testing"
	"time"
)

func TestAlarmOccurrenceSummaryReturnsRecentWithNotes(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, "", "", "check in", "every Sunday", "", "", now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	occ1, err := AlarmOccurrenceInsert(ctx, conn, alarm.ID, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("insert occurrence 1: %v", err)
	}
	err = AlarmOccurrenceUpdateNote(ctx, conn, occ1.ID, "first check-in")
	if err != nil {
		t.Fatalf("update note 1: %v", err)
	}

	_, err = AlarmOccurrenceInsert(ctx, conn, alarm.ID, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("insert occurrence 2: %v", err)
	}

	_, err = AlarmOccurrenceInsert(ctx, conn, alarm.ID, now)
	if err != nil {
		t.Fatalf("insert occurrence 3: %v", err)
	}

	occurrences, err := AlarmOccurrenceSummary(ctx, conn, alarm.ID, 5)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}

	if len(occurrences) != 3 {
		t.Fatalf("expected 3 occurrences, got %d", len(occurrences))
	}

	// ordered by fired_at DESC — most recent first
	if occurrences[2].ID != occ1.ID {
		t.Fatalf("expected oldest occurrence last")
	}
	if !occurrences[2].Note.Valid || occurrences[2].Note.String != "first check-in" {
		t.Fatalf("expected note on first occurrence: %+v", occurrences[2].Note)
	}
}

func TestAlarmOccurrenceSummaryRespectsLimit(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, "", "", "test", "", "", "", now)
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	for i := 0; i < 5; i++ {
		_, err = AlarmOccurrenceInsert(ctx, conn, alarm.ID, now.Add(time.Duration(i)*time.Hour))
		if err != nil {
			t.Fatalf("insert occurrence %d: %v", i, err)
		}
	}

	occurrences, err := AlarmOccurrenceSummary(ctx, conn, alarm.ID, 3)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}

	if len(occurrences) != 3 {
		t.Fatalf("expected 3 occurrences with limit, got %d", len(occurrences))
	}
}
