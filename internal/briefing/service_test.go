package briefing

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestHasBriefingReturnsFalseWhenNoEntry(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	has, err := svc.HasBriefing(ctx)
	if err != nil {
		t.Fatalf("has briefing: %v", err)
	}

	if has {
		t.Fatal("expected no briefing")
	}
}

func TestWriteBriefingThenHasBriefing(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.WriteBriefing(ctx, "read about AI breakthroughs")
	if err != nil {
		t.Fatalf("write briefing: %v", err)
	}

	has, err := svc.HasBriefing(ctx)
	if err != nil {
		t.Fatalf("has briefing: %v", err)
	}

	if !has {
		t.Fatal("expected briefing to exist")
	}
}

func TestWriteBriefingDuplicateErrors(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.WriteBriefing(ctx, "first briefing")
	if err != nil {
		t.Fatalf("first write: %v", err)
	}

	err = svc.WriteBriefing(ctx, "second briefing")
	if err == nil {
		t.Fatal("expected error on duplicate write")
	}
}

func TestListTopicsEmpty(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	topics, err := svc.ListTopics(ctx)
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}

	if len(topics) != 0 {
		t.Fatalf("expected 0 topics, got %d", len(topics))
	}
}

func TestAddAndListTopics(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	contactID := db.NewID()
	_, err := svc.db.ExecContext(ctx, "INSERT INTO contact (id, name) VALUES (?, ?)", contactID, "CT")
	if err != nil {
		t.Fatalf("insert contact: %v", err)
	}

	id, err := svc.AddTopic(ctx, "F1 racing news", "CT loves F1", sql.NullString{String: contactID, Valid: true})
	if err != nil {
		t.Fatalf("add topic: %v", err)
	}

	if id == "" {
		t.Fatal("expected non-empty id")
	}

	topics, err := svc.ListTopics(ctx)
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}

	if len(topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(topics))
	}

	if topics[0].Query != "F1 racing news" {
		t.Fatalf("expected query 'F1 racing news', got %q", topics[0].Query)
	}

	if topics[0].Reason != "CT loves F1" {
		t.Fatalf("expected reason 'CT loves F1', got %q", topics[0].Reason)
	}

	if !topics[0].ContactID.Valid || topics[0].ContactID.String != contactID {
		t.Fatalf("expected contact_id %q, got %v", contactID, topics[0].ContactID)
	}
}

func TestAddTopicWithoutContact(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	_, err := svc.AddTopic(ctx, "world headlines", "general awareness", sql.NullString{})
	if err != nil {
		t.Fatalf("add topic: %v", err)
	}

	topics, err := svc.ListTopics(ctx)
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}

	if topics[0].ContactID.Valid {
		t.Fatalf("expected null contact_id, got %v", topics[0].ContactID)
	}
}

func TestRemoveTopic(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	id, err := svc.AddTopic(ctx, "March Madness", "CT is following", sql.NullString{})
	if err != nil {
		t.Fatalf("add topic: %v", err)
	}

	err = svc.RemoveTopic(ctx, id)
	if err != nil {
		t.Fatalf("remove topic: %v", err)
	}

	topics, err := svc.ListTopics(ctx)
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}

	if len(topics) != 0 {
		t.Fatalf("expected 0 topics after remove, got %d", len(topics))
	}
}

func testService(t *testing.T) *Service {
	t.Helper()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	cfg := &config.Config{
		Timezone: "America/Los_Angeles",
	}

	svc := NewService(conn, cfg)
	svc.now = func() time.Time {
		return time.Date(2026, 2, 28, 9, 0, 0, 0, cfg.TZ())
	}

	return svc
}
