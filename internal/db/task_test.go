package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestTaskRetryChainAnnotatesReadReports(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC()
	rootID := id.V7()

	err = TaskInsert(ctx, conn, TaskInsertParams{
		ID:        rootID,
		Goal:      "attempt zero",
		Plan:      "plan",
		Thinking:  "low",
		Status:    "completed",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("insert root task: %v", err)
	}

	err = TaskReportInsert(ctx, conn, TaskReportInsertParams{
		ID:        id.V7(),
		TaskID:    rootID,
		Status:    "completed",
		Content:   "first report",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}

	retryID := id.V7()
	err = TaskInsert(ctx, conn, TaskInsertParams{
		ID:             retryID,
		RetryForTaskID: rootID,
		RetryNumber:    1,
		Goal:           "attempt one",
		Plan:           "better plan",
		Thinking:       "low",
		Status:         "running",
		CreatedAt:      now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("insert retry task: %v", err)
	}

	unreadReportID := id.V7()
	err = TaskReportInsert(ctx, conn, TaskReportInsertParams{
		ID:        unreadReportID,
		TaskID:    retryID,
		Status:    "running",
		Content:   "second report",
		CreatedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("insert unread report: %v", err)
	}

	chain, err := TaskRetryChain(ctx, conn, rootID)
	if err != nil {
		t.Fatalf("query retry chain: %v", err)
	}

	if len(chain) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(chain))
	}

	entry0 := chain[0]
	if entry0.ID != rootID {
		t.Fatalf("entry 0 id: want %s, got %s", rootID, entry0.ID)
	}
	if len(entry0.Reports) != 1 {
		t.Fatalf("entry 0 reports: want 1, got %d", len(entry0.Reports))
	}
	if entry0.Reports[0].Content != "first report" {
		t.Fatalf("entry 0 report content: want %q, got %q", "first report", entry0.Reports[0].Content)
	}

	entry1 := chain[1]
	if entry1.ID != retryID {
		t.Fatalf("entry 1 id: want %s, got %s", retryID, entry1.ID)
	}
	if len(entry1.Reports) != 1 {
		t.Fatalf("entry 1 reports: want 1, got %d", len(entry1.Reports))
	}
	if entry1.Reports[0].Content != "second report" {
		t.Fatalf("entry 1 report content: want %q, got %q", "second report", entry1.Reports[0].Content)
	}
}

func TestTaskRetryChainNoReports(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	rootID := id.V7()
	err = TaskInsert(ctx, conn, TaskInsertParams{
		ID:        rootID,
		Goal:      "silent task",
		Plan:      "plan",
		Thinking:  "low",
		Status:    "completed",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	chain, err := TaskRetryChain(ctx, conn, rootID)
	if err != nil {
		t.Fatalf("query retry chain: %v", err)
	}

	if len(chain) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(chain))
	}

	if len(chain[0].Reports) != 0 {
		t.Fatalf("expected 0 reports, got %d", len(chain[0].Reports))
	}
}
