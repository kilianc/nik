package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestCreateAlarmPersistsRow(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	contact := seedContactForAlarm(t, ctx, conn)
	convID := seedConversation(t, ctx, conn, "whatsapp", "alarm-conv@g.us", "group")

	fireAt := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, contact.ID, convID, "follow up", "", "message", "msg-123", fireAt)
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	if alarm.ID == "" {
		t.Fatalf("expected alarm id")
	}
	if !alarm.OriginContactID.Valid || alarm.OriginContactID.String != contact.ID {
		t.Fatalf("unexpected origin_contact_id: %+v", alarm.OriginContactID)
	}
	if !alarm.OriginConversationID.Valid || alarm.OriginConversationID.String != convID {
		t.Fatalf("unexpected origin_conversation_id: %+v", alarm.OriginConversationID)
	}
	if alarm.Goal != "follow up" {
		t.Fatalf("unexpected goal: %q", alarm.Goal)
	}
	if alarm.Recurrence.Valid {
		t.Fatalf("expected null recurrence for one-shot alarm")
	}
	if !alarm.Source.Valid || alarm.Source.String != "message" {
		t.Fatalf("unexpected source: %+v", alarm.Source)
	}
	if !alarm.SourceID.Valid || alarm.SourceID.String != "msg-123" {
		t.Fatalf("unexpected source_id: %+v", alarm.SourceID)
	}

	var (
		id              string
		originContactID sql.NullString
		originID        sql.NullString
		goal            string
		recurrence      sql.NullString
		source          sql.NullString
		sourceID        sql.NullString
		gotNextFireAt   string
	)
	err = conn.QueryRowContext(
		ctx,
		`SELECT id, origin_contact_id, origin_conversation_id, goal, recurrence, source, source_id, next_fire_at FROM alarm WHERE id = ?1`,
		alarm.ID,
	).Scan(&id, &originContactID, &originID, &goal, &recurrence, &source, &sourceID, &gotNextFireAt)
	if err != nil {
		t.Fatalf("query persisted alarm: %v", err)
	}

	if id != alarm.ID {
		t.Fatalf("expected id %q, got %q", alarm.ID, id)
	}
	if !originContactID.Valid || originContactID.String != contact.ID {
		t.Fatalf("unexpected persisted origin_contact_id: %+v", originContactID)
	}
	if !originID.Valid || originID.String != convID {
		t.Fatalf("unexpected persisted origin id: %+v", originID)
	}
	if goal != "follow up" {
		t.Fatalf("unexpected persisted goal: %q", goal)
	}
	if recurrence.Valid {
		t.Fatalf("expected null recurrence")
	}
	if !source.Valid || source.String != "message" {
		t.Fatalf("unexpected persisted source: %+v", source)
	}
	if !sourceID.Valid || sourceID.String != "msg-123" {
		t.Fatalf("unexpected persisted source_id: %+v", sourceID)
	}
}

func TestCreateAlarmWithRecurrence(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	fireAt := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, "", "", "check in", "every Sunday at 7pm", "", "", fireAt)
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	if !alarm.Recurrence.Valid || alarm.Recurrence.String != "every Sunday at 7pm" {
		t.Fatalf("unexpected recurrence: %+v", alarm.Recurrence)
	}
}

func TestCreateAlarmWithNullFKs(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	fireAt := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, "", "", "reminder", "", "", "", fireAt)
	if err != nil {
		t.Fatalf("create alarm with null FKs: %v", err)
	}

	if alarm.OriginContactID.Valid {
		t.Fatalf("expected null origin_contact_id")
	}
	if alarm.OriginConversationID.Valid {
		t.Fatalf("expected null origin_conversation_id")
	}
}

func seedContactForAlarm(t *testing.T, ctx context.Context, conn *sql.DB) Contact {
	t.Helper()

	contact, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "alarm-test@s.whatsapp.net",
		Name:          "Alarm Test",
		Phone:         "12345",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact for alarm: %v", err)
	}

	return contact
}
