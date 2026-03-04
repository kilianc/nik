package journal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestCheckReturnsNilBeforeJournalTime(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 27, 20, 0, 0, 0, time.UTC))

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 0 {
		t.Fatalf("expected no outputs before journal time, got %d", len(outputs))
	}
}

func TestCheckReturnsOutputAfterJournalTime(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 27, 23, 0, 0, 0, time.UTC))

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

func TestCheckReturnsNilWhenPageExists(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 27, 23, 0, 0, 0, time.UTC))

	err := ds.svc.WritePage(ctx, "already journaled")
	if err != nil {
		t.Fatalf("write page: %v", err)
	}

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 0 {
		t.Fatalf("expected no outputs when page exists, got %d", len(outputs))
	}
}

func TestCheckReleasesActiveOnProcessed(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 27, 23, 0, 0, 0, time.UTC))

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
	ds := testDataSource(t, time.Date(2026, 2, 27, 23, 0, 0, 0, time.UTC))

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

	err = os.WriteFile(filepath.Join(promptPath, "journal.md"), []byte("reflect on your day"), 0o644)
	if err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	cfg := &config.Config{
		Home:        promptDir,
		Timezone:    "UTC",
		JournalTime: "22:00",
	}

	svc := NewService(conn, cfg)
	svc.now = func() time.Time { return now }

	ds := NewDataSource(svc, conn, nil, cfg)
	ds.svc.now = func() time.Time { return now }

	return ds
}
