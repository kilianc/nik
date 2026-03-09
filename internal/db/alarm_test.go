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
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginContactID:      contact.ID,
		OriginConversationID: convID,
		Goal:                 "follow up",
		NextFireAt:           fireAt,
	})
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

	var (
		id              string
		originContactID sql.NullString
		originID        sql.NullString
		goal            string
		recurrence      sql.NullString
		gotNextFireAt   string
	)
	err = conn.QueryRowContext(
		ctx,
		`SELECT id, origin_contact_id, origin_conversation_id, goal, recurrence, next_fire_at FROM alarm WHERE id = ?1`,
		alarm.ID,
	).Scan(&id, &originContactID, &originID, &goal, &recurrence, &gotNextFireAt)
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
}

func TestCreateAlarmWithRecurrence(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "rec-alarm@g.us", "group")

	fireAt := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "check in",
		Recurrence:           "every Sunday at 7pm",
		NextFireAt:           fireAt,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	if !alarm.Recurrence.Valid || alarm.Recurrence.String != "every Sunday at 7pm" {
		t.Fatalf("unexpected recurrence: %+v", alarm.Recurrence)
	}
}

func TestCreateAlarmRejectsEmptyConversationID(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	fireAt := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)
	_, err = CreateAlarm(ctx, conn, CreateAlarmParams{
		Goal:       "reminder",
		NextFireAt: fireAt,
	})
	if err == nil {
		t.Fatalf("expected error for empty origin_conversation_id")
	}
}

func TestDueAlarmsReturnsOnlyActiveAndDue(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "due-alarms@g.us", "group")
	now := time.Now().UTC().Truncate(time.Second)

	dueAlarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "due",
		NextFireAt:           now.Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create due alarm: %v", err)
	}

	cancelledAlarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "cancelled",
		NextFireAt:           now.Add(-30 * time.Second),
	})
	if err != nil {
		t.Fatalf("create cancelled alarm: %v", err)
	}

	err = AlarmCancel(ctx, conn, cancelledAlarm.ID)
	if err != nil {
		t.Fatalf("cancel alarm: %v", err)
	}

	_, err = CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "future",
		NextFireAt:           now.Add(5 * time.Minute),
	})
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

func TestDueAlarmsExcludesClaimedAlarms(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "claimed-alarms@g.us", "group")
	now := time.Now().UTC().Truncate(time.Second)

	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "claimed",
		NextFireAt:           now.Add(-1 * time.Minute),
	})
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

	if len(alarms) != 0 {
		t.Fatalf("expected claimed alarm to be excluded, got %d", len(alarms))
	}
}

func TestAlarmCancelRemovesFromDueList(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "cancel-alarms@g.us", "group")
	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "cancel me",
		NextFireAt:           now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	before, err := DueAlarms(ctx, conn, now)
	if err != nil {
		t.Fatalf("due alarms before cancel: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("expected 1 due alarm, got %d", len(before))
	}

	err = AlarmCancel(ctx, conn, alarm.ID)
	if err != nil {
		t.Fatalf("cancel alarm: %v", err)
	}

	after, err := DueAlarms(ctx, conn, now)
	if err != nil {
		t.Fatalf("due alarms after cancel: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected no due alarms after cancel, got %d", len(after))
	}
}

func TestAlarmClaimSetsLastFiredAtAndKeepsNextFireAt(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "claim-alarm@g.us", "group")
	now := time.Now().UTC().Truncate(time.Second)
	fireAt := now.Add(-time.Minute)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "test",
		NextFireAt:           fireAt,
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

func TestAlarmUpdateGoal(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "update-goal@g.us", "group")
	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "old goal",
		NextFireAt:           now,
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

	convID := seedConversation(t, ctx, conn, "whatsapp", "update-rec@g.us", "group")
	now := time.Now().UTC().Truncate(time.Second)
	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "test",
		Recurrence:           "every day",
		NextFireAt:           now,
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
