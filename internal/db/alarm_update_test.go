package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestAlarmUpdateGoal(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		Goal:       "old goal",
		NextFireAt: now,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	newGoal := "new goal"
	err = AlarmUpdate(ctx, conn, alarm.ID, AlarmUpdateParams{Goal: &newGoal})
	if err != nil {
		t.Fatalf("update alarm: %v", err)
	}

	var goal string
	err = conn.QueryRowContext(ctx, `SELECT goal FROM alarm WHERE id = ?1`, alarm.ID).Scan(&goal)
	if err != nil {
		t.Fatalf("query alarm: %v", err)
	}
	if goal != "new goal" {
		t.Fatalf("expected updated goal, got %q", goal)
	}
}

func TestAlarmUpdateRecurrence(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		Goal:       "test",
		Recurrence: "every day",
		NextFireAt: now,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	newRec := "every other day"
	err = AlarmUpdate(ctx, conn, alarm.ID, AlarmUpdateParams{Recurrence: &newRec})
	if err != nil {
		t.Fatalf("update alarm: %v", err)
	}

	var recurrence sql.NullString
	err = conn.QueryRowContext(ctx, `SELECT recurrence FROM alarm WHERE id = ?1`, alarm.ID).Scan(&recurrence)
	if err != nil {
		t.Fatalf("query alarm: %v", err)
	}
	if !recurrence.Valid || recurrence.String != "every other day" {
		t.Fatalf("expected updated recurrence, got %+v", recurrence)
	}
}
