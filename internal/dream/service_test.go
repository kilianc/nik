package dream

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestHasPassReturnsFalseWhenEmpty(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	has, err := svc.HasPass(ctx, 1)
	if err != nil {
		t.Fatalf("has pass: %v", err)
	}

	if has {
		t.Fatal("expected no pass")
	}
}

func TestWriteDreamThenHasPass(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.WriteDream(ctx, 1, "memories drifting...")
	if err != nil {
		t.Fatalf("write dream: %v", err)
	}

	has, err := svc.HasPass(ctx, 1)
	if err != nil {
		t.Fatalf("has pass: %v", err)
	}

	if !has {
		t.Fatal("expected pass to exist")
	}
}

func TestWriteDreamUpsertOverwrites(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.WriteDream(ctx, 1, "first dream")
	if err != nil {
		t.Fatalf("first write: %v", err)
	}

	err = svc.WriteDream(ctx, 1, "second dream")
	if err != nil {
		t.Fatalf("upsert write: %v", err)
	}
}

func TestStartPassThenHasPass(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.StartPass(ctx, 1)
	if err != nil {
		t.Fatalf("start pass: %v", err)
	}

	has, err := svc.HasPass(ctx, 1)
	if err != nil {
		t.Fatalf("has pass: %v", err)
	}

	if !has {
		t.Fatal("expected pass to exist after start")
	}
}

func TestStartPassThenWriteOverwrites(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.StartPass(ctx, 1)
	if err != nil {
		t.Fatalf("start pass: %v", err)
	}

	err = svc.WriteDream(ctx, 1, "real content")
	if err != nil {
		t.Fatalf("write after start: %v", err)
	}

	has, err := svc.HasPass(ctx, 1)
	if err != nil {
		t.Fatalf("has pass: %v", err)
	}

	if !has {
		t.Fatal("expected pass to exist after write")
	}
}

func TestGetPassesReturnsChronological(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	err := svc.WriteDream(ctx, 1, "drift")
	if err != nil {
		t.Fatalf("write pass 1: %v", err)
	}

	err = svc.WriteDream(ctx, 2, "weave")
	if err != nil {
		t.Fatalf("write pass 2: %v", err)
	}

	passes, err := svc.GetPasses(ctx)
	if err != nil {
		t.Fatalf("get passes: %v", err)
	}

	if len(passes) != 2 {
		t.Fatalf("expected 2 passes, got %d", len(passes))
	}

	if passes[0].Pass != 1 || passes[0].Content != "drift" {
		t.Fatalf("unexpected pass 1: %+v", passes[0])
	}

	if passes[1].Pass != 2 || passes[1].Content != "weave" {
		t.Fatalf("unexpected pass 2: %+v", passes[1])
	}
}

func TestCurrentSoulEmptyWhenNone(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	soul, err := svc.CurrentSoul(ctx)
	if err != nil {
		t.Fatalf("current soul: %v", err)
	}

	if soul != "" {
		t.Fatalf("expected empty soul, got %q", soul)
	}
}

func TestWriteSoulAndRead(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	v, err := svc.WriteSoul(ctx, "## personality\nwarm and curious")
	if err != nil {
		t.Fatalf("write soul: %v", err)
	}

	if v != 1 {
		t.Fatalf("expected version 1, got %d", v)
	}

	soul, err := svc.CurrentSoul(ctx)
	if err != nil {
		t.Fatalf("current soul: %v", err)
	}

	if soul != "## personality\nwarm and curious" {
		t.Fatalf("unexpected soul: %q", soul)
	}
}

func TestWriteSoulIncrementsVersion(t *testing.T) {
	ctx := context.Background()
	svc := testService(t)

	v1, err := svc.WriteSoul(ctx, "v1 soul")
	if err != nil {
		t.Fatalf("write v1: %v", err)
	}

	v2, err := svc.WriteSoul(ctx, "v2 soul")
	if err != nil {
		t.Fatalf("write v2: %v", err)
	}

	if v1 != 1 || v2 != 2 {
		t.Fatalf("expected versions 1,2 got %d,%d", v1, v2)
	}

	soul, err := svc.CurrentSoul(ctx)
	if err != nil {
		t.Fatalf("current soul: %v", err)
	}

	if soul != "v2 soul" {
		t.Fatalf("expected latest soul, got %q", soul)
	}
}

func TestTonightReflectsPreviousDay(t *testing.T) {
	svc := testService(t)

	date := svc.tonight()
	if date != "2026-02-27" {
		t.Fatalf("expected 2026-02-27, got %s", date)
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
		Timezone:   "UTC",
		DreamStart: "02:00",
	}

	svc := NewService(conn, cfg)
	svc.now = func() time.Time {
		return time.Date(2026, 2, 28, 3, 0, 0, 0, time.UTC)
	}

	return svc
}
