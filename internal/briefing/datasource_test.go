package briefing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestCheckReturnsNilBeforeBriefingTime(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 7, 0, 0, 0, time.UTC))

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 0 {
		t.Fatalf("expected no outputs before briefing time, got %d", len(outputs))
	}
}

func TestCheckReturnsOutputAfterBriefingTime(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC))

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	if len(outputs[0].Lines) == 0 {
		t.Fatal("expected non-empty lines")
	}
}

func TestCheckReturnsNilWhenBriefingExists(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC))

	err := ds.svc.WriteBriefing(ctx, "already briefed")
	if err != nil {
		t.Fatalf("write briefing: %v", err)
	}

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 0 {
		t.Fatalf("expected no outputs when briefing exists, got %d", len(outputs))
	}
}

func TestCheckReleasesActiveOnProcessed(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC))

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	ds.mu.Lock()
	isActive := ds.active
	ds.mu.Unlock()
	if !isActive {
		t.Fatal("expected active=true after check")
	}

	err = outputs[0].Processed(ctx)
	if err != nil {
		t.Fatalf("processed: %v", err)
	}

	ds.mu.Lock()
	isActive = ds.active
	ds.mu.Unlock()
	if isActive {
		t.Fatal("expected active=false after processed")
	}
}

func TestCheckNoRetriggerAfterProcessing(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC))

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	err = outputs[0].Processing(ctx)
	if err != nil {
		t.Fatalf("processing: %v", err)
	}

	err = outputs[0].Processed(ctx)
	if err != nil {
		t.Fatalf("processed: %v", err)
	}

	outputs, err = ds.Check(ctx)
	if err != nil {
		t.Fatalf("second check: %v", err)
	}

	if len(outputs) != 0 {
		t.Fatalf("expected no outputs after processing inserted placeholder, got %d", len(outputs))
	}
}

func testDataSource(t *testing.T, now time.Time) *DataSource {
	t.Helper()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	promptDir := t.TempDir()
	promptPath := filepath.Join(promptDir, "prompts")
	err = os.MkdirAll(promptPath, 0o755)
	if err != nil {
		t.Fatalf("create prompts dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(promptPath, "briefing.md"), []byte("read the news"), 0o644)
	if err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	cfg := &config.Config{
		Home:         promptDir,
		Timezone:     "UTC",
		BriefingTime: "08:00",
	}

	svc := NewService(conn, cfg)
	svc.now = func() time.Time { return now }

	ds := NewDataSource(svc, cfg)

	return ds
}
