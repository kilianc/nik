package dream

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestCheckReturnsNilBeforeDreamStart(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 1, 0, 0, 0, time.UTC))

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 0 {
		t.Fatalf("expected no outputs before dream start, got %d", len(outputs))
	}
}

func TestCheckReturnsPass1AtDreamStart(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 2, 30, 0, 0, time.UTC))

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	if outputs[0].Meta["dream_pass"] != "1" {
		t.Fatalf("expected pass 1, got %s", outputs[0].Meta["dream_pass"])
	}

	if len(outputs[0].Lines) == 0 {
		t.Fatal("expected non-empty lines")
	}
}

func TestCheckSkipsCompletedPasses(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 3, 30, 0, 0, time.UTC))

	err := ds.svc.WriteDream(ctx, 1, "pass 1 done")
	if err != nil {
		t.Fatalf("write pass 1: %v", err)
	}

	err = ds.svc.WriteDream(ctx, 2, "pass 2 done")
	if err != nil {
		t.Fatalf("write pass 2: %v", err)
	}

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 0 {
		t.Fatalf("expected no outputs when all due passes done, got %d", len(outputs))
	}
}

func TestCheckPicksLatestDuePass(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 4, 30, 0, 0, time.UTC))

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	if outputs[0].Meta["dream_pass"] != "3" {
		t.Fatalf("expected pass 3 (latest due), got %s", outputs[0].Meta["dream_pass"])
	}
}

func TestCheckReleasesActiveOnProcessed(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 2, 30, 0, 0, time.UTC))

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

	secondOutputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("second check: %v", err)
	}
	if len(secondOutputs) != 0 {
		t.Fatal("expected no outputs while active")
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

func TestCheckPass5IsWake(t *testing.T) {
	ctx := context.Background()
	ds := testDataSource(t, time.Date(2026, 2, 28, 6, 30, 0, 0, time.UTC))

	for i := 1; i <= 4; i++ {
		err := ds.svc.WriteDream(ctx, i, "done")
		if err != nil {
			t.Fatalf("write pass %d: %v", i, err)
		}
	}

	outputs, err := ds.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	if outputs[0].Meta["dream_pass"] != "5" {
		t.Fatalf("expected pass 5, got %s", outputs[0].Meta["dream_pass"])
	}

	found := false
	for _, line := range outputs[0].Lines {
		if line == "[Wake]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected [Wake] header in output")
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

	err = os.WriteFile(filepath.Join(promptPath, "02-dream.md"), []byte("you are dreaming"), 0o644)
	if err != nil {
		t.Fatalf("write dream prompt: %v", err)
	}

	err = os.WriteFile(filepath.Join(promptPath, "03-wake.md"), []byte("you are waking up"), 0o644)
	if err != nil {
		t.Fatalf("write wake prompt: %v", err)
	}

	cfg := &config.Config{
		Home:       promptDir,
		Timezone:   "UTC",
		DreamStart: "02:00",
	}

	svc := NewService(conn, cfg)
	svc.now = func() time.Time { return now }

	ds := NewDataSource(svc, conn, nil, cfg)

	return ds
}
