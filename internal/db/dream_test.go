package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestDreamHasPassReturnsFalseWhenEmpty(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	has, err := DreamHasPass(ctx, conn, "2026-02-27", 1)
	if err != nil {
		t.Fatalf("check dream pass: %v", err)
	}

	if has {
		t.Fatal("expected no pass")
	}
}

func TestDreamWritePassPersists(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = DreamWritePass(ctx, conn, "2026-02-27", 1, "memories drift")
	if err != nil {
		t.Fatalf("write dream pass: %v", err)
	}

	has, err := DreamHasPass(ctx, conn, "2026-02-27", 1)
	if err != nil {
		t.Fatalf("check dream pass: %v", err)
	}
	if !has {
		t.Fatal("expected pass to exist after write")
	}
}

func TestDreamGetPassesReturnsOrdered(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = DreamWritePass(ctx, conn, "2026-02-27", 2, "weave")
	if err != nil {
		t.Fatalf("write pass 2: %v", err)
	}

	err = DreamWritePass(ctx, conn, "2026-02-27", 1, "drift")
	if err != nil {
		t.Fatalf("write pass 1: %v", err)
	}

	passes, err := DreamGetPasses(ctx, conn, "2026-02-27")
	if err != nil {
		t.Fatalf("get passes: %v", err)
	}

	if len(passes) != 2 {
		t.Fatalf("expected 2 passes, got %d", len(passes))
	}

	if passes[0].Pass != 1 {
		t.Fatalf("expected pass 1 first, got %d", passes[0].Pass)
	}

	if passes[1].Pass != 2 {
		t.Fatalf("expected pass 2 second, got %d", passes[1].Pass)
	}
}

func TestSoulCurrentReturnsEmptyWhenNone(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	soul, err := SoulCurrent(ctx, conn)
	if err != nil {
		t.Fatalf("get soul: %v", err)
	}

	if soul.Content != "" {
		t.Fatalf("expected empty soul, got %q", soul.Content)
	}
}

func TestSoulInsertAndRead(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	v, err := SoulInsert(ctx, conn, "warm and curious", "2026-02-27")
	if err != nil {
		t.Fatalf("insert soul: %v", err)
	}

	if v != 1 {
		t.Fatalf("expected version 1, got %d", v)
	}

	soul, err := SoulCurrent(ctx, conn)
	if err != nil {
		t.Fatalf("get soul: %v", err)
	}

	if soul.Version != 1 || soul.Content != "warm and curious" {
		t.Fatalf("unexpected soul: %+v", soul)
	}
}

func TestSoulVersionAutoIncrements(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	v1, err := SoulInsert(ctx, conn, "v1", "2026-02-27")
	if err != nil {
		t.Fatalf("insert v1: %v", err)
	}

	v2, err := SoulInsert(ctx, conn, "v2", "2026-02-28")
	if err != nil {
		t.Fatalf("insert v2: %v", err)
	}

	if v1 != 1 || v2 != 2 {
		t.Fatalf("expected versions 1,2 got %d,%d", v1, v2)
	}

	soul, err := SoulCurrent(ctx, conn)
	if err != nil {
		t.Fatalf("get soul: %v", err)
	}

	if soul.Version != 2 || soul.Content != "v2" {
		t.Fatalf("expected latest soul v2, got: %+v", soul)
	}
}

func TestJournalGetPageReturnsContent(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	err = JournalWritePage(ctx, conn, "2026-02-27", "a thoughtful day")
	if err != nil {
		t.Fatalf("write journal: %v", err)
	}

	content, err := JournalGetPage(ctx, conn, "2026-02-27")
	if err != nil {
		t.Fatalf("get journal page: %v", err)
	}

	if content != "a thoughtful day" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestJournalGetPageReturnsEmptyWhenMissing(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	content, err := JournalGetPage(ctx, conn, "2026-02-27")
	if err != nil {
		t.Fatalf("get journal page: %v", err)
	}

	if content != "" {
		t.Fatalf("expected empty, got %q", content)
	}
}

func TestMemoryRandomReturnsOlderMemories(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	memID := id.V7()
	_, err = conn.ExecContext(ctx,
		"INSERT INTO memory (id, content, created_at) VALUES (?1, ?2, datetime('now', '-30 days'))",
		memID, "old memory")
	if err != nil {
		t.Fatalf("insert old memory: %v", err)
	}

	memID2 := id.V7()
	_, err = conn.ExecContext(ctx,
		"INSERT INTO memory (id, content) VALUES (?1, ?2)",
		memID2, "new memory")
	if err != nil {
		t.Fatalf("insert new memory: %v", err)
	}

	memories, err := MemoryRandom(ctx, conn, mustParseTime("2026-02-21T00:00:00Z"), 10)
	if err != nil {
		t.Fatalf("random memories: %v", err)
	}

	if len(memories) != 1 {
		t.Fatalf("expected 1 old memory, got %d", len(memories))
	}

	if memories[0].Content != "old memory" {
		t.Fatalf("unexpected content: %q", memories[0].Content)
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
