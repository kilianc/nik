package db

import (
	"context"
	"testing"
	"time"
)

func TestSkillReflex(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	t.Run("latest returns empty for missing skill", func(t *testing.T) {
		record, firedAt, err := SkillReflexLatest(ctx, conn, "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record != "" {
			t.Fatalf("expected empty record, got %q", record)
		}
		if !firedAt.IsZero() {
			t.Fatalf("expected zero time, got %v", firedAt)
		}
	})

	t.Run("insert and latest", func(t *testing.T) {
		err := SkillReflexInsert(ctx, conn, "gmail", `{"cursor":"2026-03-12T10:00:00Z"}`)
		if err != nil {
			t.Fatalf("first insert: %v", err)
		}

		record, firedAt, err := SkillReflexLatest(ctx, conn, "gmail")
		if err != nil {
			t.Fatalf("latest after first insert: %v", err)
		}
		if record != `{"cursor":"2026-03-12T10:00:00Z"}` {
			t.Fatalf("expected first record, got %q", record)
		}
		if firedAt.IsZero() {
			t.Fatal("expected non-zero firedAt")
		}

		before := time.Now()
		err = SkillReflexInsert(ctx, conn, "gmail", `{"cursor":"2026-03-12T11:00:00Z"}`)
		if err != nil {
			t.Fatalf("second insert: %v", err)
		}

		record, firedAt, err = SkillReflexLatest(ctx, conn, "gmail")
		if err != nil {
			t.Fatalf("latest after second insert: %v", err)
		}
		if record != `{"cursor":"2026-03-12T11:00:00Z"}` {
			t.Fatalf("expected second record, got %q", record)
		}
		if firedAt.Before(before.Add(-time.Second)) {
			t.Fatalf("expected firedAt near now, got %v", firedAt)
		}
	})

	t.Run("multiple skills isolated", func(t *testing.T) {
		err := SkillReflexInsert(ctx, conn, "calendar", "calendar-record")
		if err != nil {
			t.Fatalf("insert calendar: %v", err)
		}

		gmail, _, err := SkillReflexLatest(ctx, conn, "gmail")
		if err != nil {
			t.Fatalf("latest gmail: %v", err)
		}
		if gmail != `{"cursor":"2026-03-12T11:00:00Z"}` {
			t.Fatalf("expected gmail record, got %q", gmail)
		}

		calendar, _, err := SkillReflexLatest(ctx, conn, "calendar")
		if err != nil {
			t.Fatalf("latest calendar: %v", err)
		}
		if calendar != "calendar-record" {
			t.Fatalf("expected calendar-record, got %q", calendar)
		}
	})
}
