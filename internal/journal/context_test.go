package journal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

func TestBuildDayContextEmptyDB(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now()
	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	lines := buildDayContext(ctx, conn, nil, dayStart, dayEnd)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "No conversations today") {
		t.Fatal("expected 'No conversations today' in empty context")
	}
}

func TestBuildDayContextIncludesMemories(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	id := db.NewID()
	_, err = conn.ExecContext(ctx, "INSERT INTO memory (id, content) VALUES (?1, ?2)", id, "nik likes dogs")
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	now := time.Now()
	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	lines := buildDayContext(ctx, conn, nil, dayStart, dayEnd)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "nik likes dogs") {
		t.Fatal("expected memory content in day context")
	}
	if !strings.Contains(joined, "Memories formed today") {
		t.Fatal("expected memories section header")
	}
}
