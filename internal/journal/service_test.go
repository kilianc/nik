package journal

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestHasPageReturnsFalseWhenNoEntry(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	has, err := svc.HasPage(ctx)
	if err != nil {
		t.Fatalf("has page: %v", err)
	}

	if has {
		t.Fatal("expected no page")
	}
}

func TestWritePageThenHasPage(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.WritePage(ctx, "a quiet day")
	if err != nil {
		t.Fatalf("write page: %v", err)
	}

	has, err := svc.HasPage(ctx)
	if err != nil {
		t.Fatalf("has page: %v", err)
	}

	if !has {
		t.Fatal("expected page to exist")
	}
}

func TestWritePageDuplicateErrors(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.WritePage(ctx, "first entry")
	if err != nil {
		t.Fatalf("first write: %v", err)
	}

	err = svc.WritePage(ctx, "second entry")
	if err == nil {
		t.Fatal("expected error on duplicate write")
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
		// just past midnight on Feb 28 → journal reflects on Feb 27
		return time.Date(2026, 2, 28, 0, 1, 0, 0, cfg.TZ())
	}

	return svc
}
