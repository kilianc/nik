package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestAlarmClaimSetsLastFiredAtAndKeepsNextFireAt(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC().Truncate(time.Second)
	fireAt := now.Add(-time.Minute)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		Goal:       "test",
		NextFireAt: fireAt,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	err = AlarmClaim(ctx, conn, alarm.ID, now)
	if err != nil {
		t.Fatalf("claim alarm: %v", err)
	}

	var (
		nextFireAt  sql.NullString
		lastFiredAt sql.NullString
	)
	err = conn.QueryRowContext(ctx,
		`SELECT next_fire_at, last_fired_at FROM alarm WHERE id = ?1`,
		alarm.ID,
	).Scan(&nextFireAt, &lastFiredAt)
	if err != nil {
		t.Fatalf("query alarm: %v", err)
	}

	if !nextFireAt.Valid {
		t.Fatalf("expected next_fire_at to be preserved after claim")
	}
	if !lastFiredAt.Valid {
		t.Fatalf("expected last_fired_at to be set after claim")
	}
}
