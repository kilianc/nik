package apisvc

import (
	"context"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

// Counting is nikd's job now — this used to live in the TUI, against a
// database the TUI had opened for itself.
func TestWorkloadCountsAlarmsAndTasks(t *testing.T) {
	_, conn := newTestChat(t)
	ctx := context.Background()

	counts, err := NewWorkload(conn).Counts(ctx)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if counts.Alarms != 0 || counts.Tasks != 0 {
		t.Fatalf("fresh nik has %+v, want zeroes", counts)
	}

	_, err = db.AlarmCreate(ctx, conn, db.AlarmCreateParams{
		OriginConversationID: db.LocalConversationID,
		Goal:                 "ring me",
		NextFireAt:           time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("seed alarm: %v", err)
	}

	err = db.TaskInsert(ctx, conn, db.TaskInsertParams{
		ID:             "task-1",
		ConversationID: db.LocalConversationID,
		Goal:           "do thing",
		Plan:           "p",
		Thinking:       "low",
		Status:         "running",
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	counts, err = NewWorkload(conn).Counts(ctx)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if counts.Alarms != 1 {
		t.Errorf("alarms = %d, want 1", counts.Alarms)
	}
	if counts.Tasks != 1 {
		t.Errorf("tasks = %d, want 1", counts.Tasks)
	}
}
