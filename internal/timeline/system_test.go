package timeline

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/db"
)

func TestRenderMediaProcessed(t *testing.T) {
	t.Run("with reference", func(t *testing.T) {
		conn, convID := setupTestDB(t)

		now := time.Date(2026, 3, 14, 10, 30, 15, 0, time.UTC)
		insertMsg(t, conn, convID, "media-msg-id", "ext-media-msg", "image", "(image) sunset", now)

		bump := db.Message{
			Platform:        "system",
			Kind:            "media_processed",
			Body:            `{"file_path":"media/abc.jpg"}`,
			SentAt:          now.Add(time.Second),
			ContextStanzaID: sql.NullString{Valid: true, String: "media-msg-id"},
		}

		e := renderMediaProcessed(bump, conn)

		if e.from != "system" {
			t.Fatalf("expected from=system, got %q", e.from)
		}
		if !strings.Contains(e.text, "[media described]") {
			t.Fatalf("expected [media described] in text, got %q", e.text)
		}
		if !strings.Contains(e.text, "(from [10:30:15] Sender: (image) sunset)") {
			t.Fatalf("expected reference to original message, got %q", e.text)
		}
	})

	t.Run("no reference", func(t *testing.T) {
		bump := db.Message{
			Platform: "system",
			Kind:     "media_processed",
			Body:     `{"file_path":"media/abc.jpg"}`,
			SentAt:   time.Now(),
		}

		e := renderMediaProcessed(bump, nil)

		if e.text != "[media described]" {
			t.Fatalf("expected bare [media described], got %q", e.text)
		}
	})
}

func skillMessage(kind string, skill db.Skill) db.Message {
	body, _ := json.Marshal(skill)
	return db.Message{
		Platform: "system",
		Kind:     kind,
		Body:     string(body),
		SentAt:   time.Now(),
	}
}

func TestRenderSkillEvent(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		skill      db.Skill
		wantSubs   []string
		wantAbsent []string
	}{
		{
			"added with install",
			"skill_added",
			db.Skill{Name: "breathing", InstallHash: sql.NullString{String: "abc123", Valid: true}},
			[]string{"MANDATORY", "load_skill", "[Skill added]"},
			nil,
		},
		{
			"added without install",
			"skill_added",
			db.Skill{Name: "shell"},
			[]string{"[Skill added]", "shell"},
			[]string{"MANDATORY"},
		},
		{
			"changed with install",
			"skill_changed",
			db.Skill{Name: "journal", InstallHash: sql.NullString{String: "def456", Valid: true}},
			[]string{"MANDATORY", "idempotently", "no duplicate alarms"},
			nil,
		},
		{
			"changed without install",
			"skill_changed",
			db.Skill{Name: "config"},
			nil,
			[]string{"MANDATORY"},
		},
		{
			"removed",
			"skill_removed",
			db.Skill{Name: "journal"},
			[]string{"[Skill removed]", "ask user"},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := skillMessage(tt.kind, tt.skill)
			e := renderSkillEvent(msg)

			for _, sub := range tt.wantSubs {
				if !strings.Contains(e.text, sub) {
					t.Fatalf("expected %q in output, got: %s", sub, e.text)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(e.text, absent) {
					t.Fatalf("expected %q absent from output, got: %s", absent, e.text)
				}
			}
		})
	}
}

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
		{
			name:     "trigger",
			kind:     "trigger",
			body:     map[string]string{"skill": "breathing"},
			wantFrom: "system",
			wantSub:  "[Trigger] load breathing skill",
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
