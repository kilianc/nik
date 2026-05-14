package db

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestTaskRetryChain(t *testing.T) {
	t.Run("annotates read reports", func(t *testing.T) {
		ctx := context.Background()

		conn, err := OpenInMemory()
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer conn.Close()

		now := time.Now().UTC()
		rootID := id.V7()
		convID := seedConversation(t, ctx, conn, "whatsapp", "task-retry-chain@s.whatsapp.net", "dm")

		err = TaskInsert(ctx, conn, TaskInsertParams{
			ID:             rootID,
			ConversationID: convID,
			Goal:           "attempt zero",
			Plan:           "plan",
			Thinking:       "low",
			Status:         "completed",
			CreatedAt:      now,
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
			ConversationID: convID,
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
	})

	t.Run("update plan", func(t *testing.T) {
		ctx := context.Background()

		conn, err := OpenInMemory()
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer conn.Close()

		taskID := id.V7()
		convID := seedConversation(t, ctx, conn, "whatsapp", "plan-update@s.whatsapp.net", "dm")

		err = TaskInsert(ctx, conn, TaskInsertParams{
			ID:             taskID,
			ConversationID: convID,
			Goal:           "test plan update",
			Plan:           "- [ ] step one\n- [ ] step two",
			Thinking:       "low",
			Status:         "running",
			CreatedAt:      time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("insert task: %v", err)
		}

		updatedPlan := "- [x] step one\n- [>] step two\n  - [ ] substep added"
		err = TaskUpdate(ctx, conn, TaskUpdateParams{
			ID:   taskID,
			Plan: &updatedPlan,
		})
		if err != nil {
			t.Fatalf("update plan: %v", err)
		}

		got, err := TaskGet(ctx, conn, taskID)
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if got.Plan != updatedPlan {
			t.Errorf("plan = %q, want %q", got.Plan, updatedPlan)
		}
		if got.Status != "running" {
			t.Errorf("status changed unexpectedly: got %q", got.Status)
		}
	})

	t.Run("no reports", func(t *testing.T) {
		ctx := context.Background()

		conn, err := OpenInMemory()
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer conn.Close()

		rootID := id.V7()
		convID := seedConversation(t, ctx, conn, "whatsapp", "silent-task@s.whatsapp.net", "dm")
		err = TaskInsert(ctx, conn, TaskInsertParams{
			ID:             rootID,
			ConversationID: convID,
			Goal:           "silent task",
			Plan:           "plan",
			Thinking:       "low",
			Status:         "completed",
			CreatedAt:      time.Now().UTC(),
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
	})
}

func TestTaskCountActiveCountsPendingAndRunning(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	convID := seedConversation(t, ctx, conn, "whatsapp", "task-count@s.whatsapp.net", "dm")
	now := time.Now().UTC()

	n, err := TaskCountActive(ctx, conn)
	if err != nil {
		t.Fatalf("count empty: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 active tasks on empty db, got %d", n)
	}

	cases := []struct {
		status string
		active bool
	}{
		{"pending", true},
		{"running", true},
		{"completed", false},
		{"failed", false},
		{"cancelled", false},
	}

	want := 0
	for _, c := range cases {
		err = TaskInsert(ctx, conn, TaskInsertParams{
			ID:             id.V7(),
			ConversationID: convID,
			Goal:           "g-" + c.status,
			Plan:           "p",
			Thinking:       "low",
			Status:         c.status,
			CreatedAt:      now,
		})
		if err != nil {
			t.Fatalf("insert %s: %v", c.status, err)
		}
		if c.active {
			want++
		}
	}

	n, err = TaskCountActive(ctx, conn)
	if err != nil {
		t.Fatalf("count after inserts: %v", err)
	}
	if n != want {
		t.Errorf("expected %d active tasks, got %d", want, n)
	}
}
