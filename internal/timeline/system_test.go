package timeline

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/db"
)

func TestRenderSystemMessage(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		kind     string
		body     any
		wantFrom string
		wantSub  string
	}{
		{
			name:     "task_report",
			kind:     "task_report",
			body:     db.TaskReport{TaskID: "aaaa-bbbb-cccc-dddd", Goal: "do thing", Status: "running", Content: "working on it"},
			wantFrom: "task",
			wantSub:  "[Task report]",
		},
		{
			name:     "task_spawned",
			kind:     "task_spawned",
			body:     db.Task{ID: "aaaa-bbbb-cccc-dddd", Goal: "do thing"},
			wantFrom: "system",
			wantSub:  "[Task spawned]",
		},
		{
			name:     "task_cancelled",
			kind:     "task_cancelled",
			body:     db.Task{ID: "aaaa-bbbb-cccc-dddd", Goal: "do thing"},
			wantFrom: "system",
			wantSub:  "[Task cancelled]",
		},
		{
			name:     "alarm_fired",
			kind:     "alarm_fired",
			body:     db.Alarm{ID: "aaaa-bbbb-cccc-dddd", Goal: "check email"},
			wantFrom: "alarm",
			wantSub:  "[One-off alarm fired]",
		},
		{
			name:     "alarm_stale",
			kind:     "alarm_stale",
			body:     db.Alarm{ID: "aaaa-bbbb-cccc-dddd", Goal: "check email"},
			wantFrom: "alarm",
			wantSub:  "[Alarm needs rescheduling]",
		},
		{
			name:     "alarm_created",
			kind:     "alarm_created",
			body:     db.Alarm{ID: "aaaa-bbbb-cccc-dddd", Goal: "check email"},
			wantFrom: "system",
			wantSub:  "[Alarm created]",
		},
		{
			name:     "alarm_updated",
			kind:     "alarm_updated",
			body:     alarms.AlarmUpdated{Alarm: db.Alarm{ID: "aaaa-bbbb-cccc-dddd", Goal: "check email"}, Note: "done"},
			wantFrom: "system",
			wantSub:  "note: done",
		},
		{
			name:     "skill_added",
			kind:     "skill_added",
			body:     db.Skill{Name: "journal", Status: "active"},
			wantFrom: "skill",
			wantSub:  "[Skill added]",
		},
		{
			name:     "skill_removed",
			kind:     "skill_removed",
			body:     db.Skill{Name: "journal", Status: "removed"},
			wantFrom: "skill",
			wantSub:  "[Skill removed]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyJSON, _ := json.Marshal(tt.body)
			msg := db.Message{
				Kind:   tt.kind,
				Body:   string(bodyJSON),
				SentAt: now,
			}

			e := renderSystemMessage(msg)

			if e.from != tt.wantFrom {
				t.Errorf("from = %q, want %q", e.from, tt.wantFrom)
			}
			if !strings.Contains(e.text, tt.wantSub) {
				t.Errorf("text = %q, want substring %q", e.text, tt.wantSub)
			}
		})
	}
}
