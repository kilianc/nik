package alarms

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestNewServiceStoresDB(t *testing.T) {
	svc := New(nil)
	if svc == nil {
		t.Fatalf("expected non-nil service")
	}
	if svc.db != nil {
		t.Fatalf("expected nil db when initialized with nil")
	}
}

func TestCreateAlarmRejectsInvalidTimestamp(t *testing.T) {
	svc := New(nil)

	_, err := svc.CreateAlarm(context.Background(), "kevin", "", "wake up", "", "not-a-time")
	if err == nil {
		t.Fatalf("expected parse error")
	}

	if !strings.Contains(err.Error(), "parse next_fire_at") {
		t.Fatalf("expected parse next_fire_at error, got %v", err)
	}
}

func TestHealStaleAlarmsDoesNotWriteToDB(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn)
	now := time.Now().UTC().Truncate(time.Second)

	alarm, err := db.CreateAlarm(ctx, conn, db.CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "[NIK_JOURNAL] End of day journal",
		Recurrence:           "every day",
		NextFireAt:           now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	err = db.AlarmUpdate(ctx, conn, alarm.ID, db.AlarmUpdateParams{LastFiredAt: now.Add(-time.Hour)})
	if err != nil {
		t.Fatalf("claim alarm: %v", err)
	}

	svc := New(conn)
	svc.healStaleAlarms(ctx)

	var lastFiredAt sql.NullString
	err = conn.QueryRowContext(ctx,
		`SELECT last_fired_at FROM alarm WHERE id = ?1`, alarm.ID,
	).Scan(&lastFiredAt)
	if err != nil {
		t.Fatalf("query alarm: %v", err)
	}
	if !lastFiredAt.Valid {
		t.Fatalf("expected last_fired_at to be preserved (detection only), got NULL")
	}
}

func TestHealStaleAlarmsIgnoresHealthy(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn)
	now := time.Now().UTC().Truncate(time.Second)

	_, err = db.CreateAlarm(ctx, conn, db.CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "[NIK_JOURNAL] End of day journal",
		Recurrence:           "every day",
		NextFireAt:           now.Add(12 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	svc := New(conn)
	svc.healStaleAlarms(ctx)

	occs, err := db.AlarmOccurrenceList(ctx, conn, convID, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("list occurrences: %v", err)
	}

	if len(occs) != 0 {
		t.Fatalf("expected no occurrences for healthy alarm, got %d", len(occs))
	}
}

func seedConversation(t *testing.T, ctx context.Context, conn *sql.DB) string {
	t.Helper()

	now := time.Now()
	err := db.UpsertConversation(ctx, conn, db.UpsertConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "stale-alarm-test@s.whatsapp.net",
		Kind:                   "dm",
		LastMessageAt:          &now,
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	conv, err := db.GetConversation(ctx, conn, db.GetConversationParams{
		Platform:               "whatsapp",
		ExternalConversationID: "stale-alarm-test@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	return conv.ID
}
