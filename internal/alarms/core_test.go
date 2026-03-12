package alarms

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestNextDailyFireAtFutureToday(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 3, 11, 6, 0, 0, 0, tz)

	got, err := nextDailyFireAt("08:00", 0, tz, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := time.Date(2026, 3, 11, 8, 0, 0, 0, tz)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNextDailyFireAtPastAdvancesToTomorrow(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, tz)

	got, err := nextDailyFireAt("08:00", 0, tz, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := time.Date(2026, 3, 12, 8, 0, 0, 0, tz)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNextDailyFireAtWithOffset(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 3, 11, 1, 0, 0, 0, tz)

	got, err := nextDailyFireAt("02:00", 3*time.Hour, tz, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := time.Date(2026, 3, 11, 5, 0, 0, 0, tz)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNextDailyFireAtWithOffsetPastAdvances(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 3, 11, 6, 0, 0, 0, tz)

	got, err := nextDailyFireAt("02:00", 3*time.Hour, tz, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := time.Date(2026, 3, 12, 5, 0, 0, 0, tz)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNextDailyFireAtInvalidFormat(t *testing.T) {
	_, err := nextDailyFireAt("bad", 0, time.UTC, time.Now())
	if err == nil {
		t.Fatalf("expected error for invalid time format")
	}
}

func TestEnsureCoreAlarmsCreatesWhenMissing(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn)
	cfg := &config.Config{
		DiagnosticTime:            "08:00",
		Timezone:                  "UTC",
		PrivilegedConversationIDs: []string{convID},
	}

	svc := New(conn)
	svc.ensureCoreAlarms(ctx, cfg)

	alarm, ok, err := db.AlarmGet(ctx, conn, "[NIK_DIAGNOSTIC]")
	if err != nil {
		t.Fatalf("find alarm: %v", err)
	}
	if !ok {
		t.Fatalf("expected diagnostic alarm to be created")
	}
	if !alarm.NextFireAt.Valid {
		t.Fatalf("expected next_fire_at to be set")
	}
	if !alarm.NextFireAt.Time.After(time.Now()) {
		t.Fatalf("expected next_fire_at to be in the future")
	}
}

func TestEnsureCoreAlarmsHealsDeadAlarm(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn)
	cfg := &config.Config{
		BriefingTime:              "08:00",
		Timezone:                  "UTC",
		PrivilegedConversationIDs: []string{convID},
	}

	pastTime := time.Now().Add(-24 * time.Hour)
	_, err = db.CreateAlarm(ctx, conn, db.CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "[NIK_BRIEFING] Morning briefing -- load briefing skill",
		Recurrence:           "every day",
		NextFireAt:           pastTime,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	svc := New(conn)
	svc.ensureCoreAlarms(ctx, cfg)

	alarm, ok, err := db.AlarmGet(ctx, conn, "[NIK_BRIEFING]")
	if err != nil {
		t.Fatalf("find alarm: %v", err)
	}
	if !ok {
		t.Fatalf("expected alarm to exist")
	}
	if !alarm.NextFireAt.Time.After(time.Now()) {
		t.Fatalf("expected next_fire_at to be healed to future, got %v", alarm.NextFireAt.Time)
	}
}

func TestEnsureCoreAlarmsSkipsHealthy(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn)
	cfg := &config.Config{
		JournalTime:               "00:00",
		Timezone:                  "UTC",
		PrivilegedConversationIDs: []string{convID},
	}

	futureTime := time.Now().Add(12 * time.Hour)
	created, err := db.CreateAlarm(ctx, conn, db.CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "[NIK_JOURNAL] End of day journal -- load journal skill",
		Recurrence:           "every day",
		NextFireAt:           futureTime,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	svc := New(conn)
	svc.ensureCoreAlarms(ctx, cfg)

	alarm, ok, err := db.AlarmGet(ctx, conn, "[NIK_JOURNAL]")
	if err != nil {
		t.Fatalf("find alarm: %v", err)
	}
	if !ok {
		t.Fatalf("expected alarm to exist")
	}
	if alarm.ID != created.ID {
		t.Fatalf("expected same alarm id, got different")
	}
}

func TestEnsureCoreAlarmsSkipsEmptyConfigTime(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn)
	cfg := &config.Config{
		Timezone:                  "UTC",
		PrivilegedConversationIDs: []string{convID},
	}

	svc := New(conn)
	svc.ensureCoreAlarms(ctx, cfg)

	_, ok, err := db.AlarmGet(ctx, conn, "[NIK_DIAGNOSTIC]")
	if err != nil {
		t.Fatalf("find alarm: %v", err)
	}
	if ok {
		t.Fatalf("expected no alarm when config time is empty")
	}
}

func TestEnsureCoreAlarmsSkipsNoPrivilegedConversation(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	cfg := &config.Config{
		DiagnosticTime: "08:00",
		Timezone:       "UTC",
	}

	svc := New(conn)
	svc.ensureCoreAlarms(ctx, cfg)

	_, ok, err := db.AlarmGet(ctx, conn, "[NIK_DIAGNOSTIC]")
	if err != nil {
		t.Fatalf("find alarm: %v", err)
	}
	if ok {
		t.Fatalf("expected no alarm when no privileged conversation")
	}
}

func seedConversation(t *testing.T, ctx context.Context, conn *sql.DB) string {
	t.Helper()

	now := time.Now()
	err := db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "core-alarm-test@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "core-alarm-test@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	return conv.ID
}
