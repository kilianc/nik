package db

import (
	"context"
	"testing"
	"time"
)

func TestAlarmGetByGoalPrefix(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "alarm-get@g.us", "group")
	fireAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	created, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "[NIK_DIAGNOSTIC] System diagnostic -- load diagnostic skill",
		Recurrence:           "every day",
		NextFireAt:           fireAt,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	found, ok, err := AlarmGet(ctx, conn, "[NIK_DIAGNOSTIC]")
	if err != nil {
		t.Fatalf("alarm get by prefix: %v", err)
	}
	if !ok {
		t.Fatalf("expected alarm to be found")
	}
	if found.ID != created.ID {
		t.Fatalf("expected id %q, got %q", created.ID, found.ID)
	}
}

func TestAlarmGetByID(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "alarm-get-id@g.us", "group")
	fireAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	created, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "some alarm goal",
		NextFireAt:           fireAt,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	found, ok, err := AlarmGet(ctx, conn, created.ID)
	if err != nil {
		t.Fatalf("alarm get by id: %v", err)
	}
	if !ok {
		t.Fatalf("expected alarm to be found by id")
	}
	if found.Goal != "some alarm goal" {
		t.Fatalf("expected goal %q, got %q", "some alarm goal", found.Goal)
	}
}

func TestAlarmGetNoMatch(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "alarm-get-no@g.us", "group")
	fireAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	_, err = CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "[NIK_JOURNAL] End of day journal",
		NextFireAt:           fireAt,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	_, ok, err := AlarmGet(ctx, conn, "[NIK_DIAGNOSTIC]")
	if err != nil {
		t.Fatalf("alarm get: %v", err)
	}
	if ok {
		t.Fatalf("expected no match for different prefix")
	}
}

func TestAlarmGetIgnoresCancelled(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "alarm-get-cancel@g.us", "group")
	fireAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	alarm, err := CreateAlarm(ctx, conn, CreateAlarmParams{
		OriginConversationID: convID,
		Goal:                 "[NIK_BRIEFING] Morning briefing",
		NextFireAt:           fireAt,
	})
	if err != nil {
		t.Fatalf("create alarm: %v", err)
	}

	err = AlarmCancel(ctx, conn, alarm.ID)
	if err != nil {
		t.Fatalf("cancel alarm: %v", err)
	}

	_, ok, err := AlarmGet(ctx, conn, "[NIK_BRIEFING]")
	if err != nil {
		t.Fatalf("alarm get: %v", err)
	}
	if ok {
		t.Fatalf("expected cancelled alarm to be excluded")
	}
}
