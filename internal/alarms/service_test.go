package alarms

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestFireDueAlarmsClearsLastOccurrenceNote(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	err = db.EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn)
	now := time.Now().UTC().Truncate(time.Second)

	alarm, err := db.CreateAlarm(ctx, conn, db.CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "journal",
		NextFireAt:           now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	err = db.AlarmUpdate(ctx, conn, alarm.ID, db.AlarmUpdateParams{
		ApplyLastOccurrenceNote: true,
		LastOccurrenceNote:      "old note",
	})
	if err != nil {
		t.Fatalf("seed last occurrence note: %v", err)
	}

	svc := New(nil, conn)
	svc.FireDueAlarms(ctx)

	updated, found, err := db.AlarmGet(ctx, conn, alarm.ID)
	if err != nil {
		t.Fatalf("get alarm: %v", err)
	}
	if !found {
		t.Fatalf("expected alarm to exist")
	}
	if updated.LastOccurrenceNote.Valid {
		t.Fatalf("expected last_occurrence_note to be cleared, got %+v", updated.LastOccurrenceNote)
	}

	var msgCount int
	err = conn.QueryRowContext(ctx,
		`SELECT count(*) FROM message WHERE conversation_id = ?1 AND kind = 'alarm_fired'`,
		convID,
	).Scan(&msgCount)
	if err != nil {
		t.Fatalf("count fired messages: %v", err)
	}
	if msgCount != 1 {
		t.Fatalf("expected 1 alarm_fired message, got %d", msgCount)
	}
}

func TestUpdateAlarmNoteWritesBothStores(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	err = db.EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn)
	now := time.Now().UTC().Truncate(time.Second)

	alarm, err := db.CreateAlarm(ctx, conn, db.CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "journal",
		NextFireAt:           now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	_, err = db.AlarmOccurrenceInsert(ctx, conn, alarm.ID, now)
	if err != nil {
		t.Fatalf("insert occurrence: %v", err)
	}

	svc := New(nil, conn)
	err = svc.Update(ctx, alarm.ID, UpdateParams{Note: "done and archived"})
	if err != nil {
		t.Fatalf("update note: %v", err)
	}

	updated, found, err := db.AlarmGet(ctx, conn, alarm.ID)
	if err != nil {
		t.Fatalf("get alarm: %v", err)
	}
	if !found {
		t.Fatalf("expected alarm to exist")
	}
	if !updated.LastOccurrenceNote.Valid || updated.LastOccurrenceNote.String != "done and archived" {
		t.Fatalf("expected last_occurrence_note to be updated, got %+v", updated.LastOccurrenceNote)
	}

	var occurrenceNote sql.NullString
	err = conn.QueryRowContext(ctx,
		`SELECT note FROM alarm_occurrence WHERE alarm_id = ?1 ORDER BY fired_at DESC LIMIT 1`,
		alarm.ID,
	).Scan(&occurrenceNote)
	if err != nil {
		t.Fatalf("query latest occurrence note: %v", err)
	}
	if !occurrenceNote.Valid || occurrenceNote.String != "done and archived" {
		t.Fatalf("expected latest occurrence note to be updated, got %+v", occurrenceNote)
	}

	var msgNote string
	err = conn.QueryRowContext(ctx,
		`SELECT json_extract(body, '$.note')
		 FROM message
		 WHERE conversation_id = ?1
		   AND kind = 'alarm_updated'
		 ORDER BY sent_at DESC
		 LIMIT 1`,
		convID,
	).Scan(&msgNote)
	if err != nil {
		t.Fatalf("query alarm_updated note: %v", err)
	}
	if msgNote != "done and archived" {
		t.Fatalf("expected alarm_updated note %q, got %q", "done and archived", msgNote)
	}
}

func TestUpdateAlarmNoteBeforeFirstOccurrenceReturnsError(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	err = db.EnsureSystemContact(ctx, conn)
	if err != nil {
		t.Fatalf("ensure system contact: %v", err)
	}

	convID := seedConversation(t, ctx, conn)
	now := time.Now().UTC().Truncate(time.Second)

	alarm, err := db.CreateAlarm(ctx, conn, db.CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "journal",
		NextFireAt:           now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	svc := New(nil, conn)
	err = svc.Update(ctx, alarm.ID, UpdateParams{Note: "premature"})
	if err == nil {
		t.Fatalf("expected error before first occurrence")
	}

	if !strings.Contains(err.Error(), "has no occurrence") {
		t.Fatalf("expected occurrence error, got %v", err)
	}
}

func TestHealStaleAlarms(t *testing.T) {
	t.Run("emits system message for stale alarm", func(t *testing.T) {
		ctx := context.Background()

		conn, err := db.OpenInMemory()
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer conn.Close()

		err = db.EnsureSystemContact(ctx, conn)
		if err != nil {
			t.Fatalf("ensure system contact: %v", err)
		}

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

		svc := New(nil, conn)
		svc.healStaleAlarms(ctx)

		var msgCount int
		err = conn.QueryRowContext(ctx,
			`SELECT count(*) FROM message WHERE conversation_id = ?1 AND kind = 'alarm_stale'`,
			convID,
		).Scan(&msgCount)
		if err != nil {
			t.Fatalf("count stale messages: %v", err)
		}
		if msgCount != 1 {
			t.Fatalf("expected 1 stale message, got %d", msgCount)
		}
	})

	t.Run("ignores healthy alarm", func(t *testing.T) {
		ctx := context.Background()

		conn, err := db.OpenInMemory()
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer conn.Close()

		err = db.EnsureSystemContact(ctx, conn)
		if err != nil {
			t.Fatalf("ensure system contact: %v", err)
		}

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

		svc := New(nil, conn)
		svc.healStaleAlarms(ctx)

		var msgCount int
		err = conn.QueryRowContext(ctx,
			`SELECT count(*) FROM message WHERE conversation_id = ?1 AND kind = 'alarm_stale'`,
			convID,
		).Scan(&msgCount)
		if err != nil {
			t.Fatalf("count stale messages: %v", err)
		}
		if msgCount != 0 {
			t.Fatalf("expected no stale messages for healthy alarm, got %d", msgCount)
		}
	})
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
