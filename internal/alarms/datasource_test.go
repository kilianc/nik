package alarms

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestFormatAlarmFreshOneShot(t *testing.T) {
	alarm := Alarm{
		ID:              "alarm-1",
		Goal:            "send follow-up",
		OriginContactID: sql.NullString{Valid: true, String: "contact-1"},
	}

	lines := formatAlarm(alarm, "kevin", nil, nil, nil, false)
	out := strings.Join(lines, "\n")

	if !strings.Contains(out, "[Alarm fired]") {
		t.Fatalf("expected one-shot header, got %q", out)
	}
	if !strings.Contains(out, "Goal: send follow-up") {
		t.Fatalf("expected goal line, got %q", out)
	}
	if !strings.Contains(out, "Requested by: kevin") {
		t.Fatalf("expected requester line, got %q", out)
	}
	if !strings.Contains(out, "Act on this now.") {
		t.Fatalf("expected action prompt, got %q", out)
	}
	if !strings.Contains(out, "cancel_alarm") {
		t.Fatalf("expected cancel instruction, got %q", out)
	}
}

func TestFormatAlarmFreshRecurring(t *testing.T) {
	alarm := Alarm{
		ID:         "alarm-2",
		Goal:       "check in with Mom",
		Recurrence: sql.NullString{Valid: true, String: "every Sunday afternoon"},
	}

	lines := formatAlarm(alarm, "", nil, nil, nil, false)
	out := strings.Join(lines, "\n")

	if !strings.Contains(out, "[Recurring alarm fired]") {
		t.Fatalf("expected recurring header, got %q", out)
	}
	if !strings.Contains(out, "Recurrence: every Sunday afternoon") {
		t.Fatalf("expected recurrence line, got %q", out)
	}
	if !strings.Contains(out, "occurrence_note") {
		t.Fatalf("expected occurrence_note instruction, got %q", out)
	}
	if !strings.Contains(out, "next_fire_at") {
		t.Fatalf("expected next_fire_at instruction, got %q", out)
	}
}

func TestFormatAlarmAlreadyFiredRecurring(t *testing.T) {
	now := time.Now()
	alarm := Alarm{
		ID:          "alarm-3",
		Goal:        "weekly report",
		Recurrence:  sql.NullString{Valid: true, String: "every Monday at 9am"},
		NextFireAt:  sql.NullTime{Valid: true, Time: now.Add(-time.Hour)},
		LastFiredAt: sql.NullTime{Valid: true, Time: now},
	}

	lines := formatAlarm(alarm, "", nil, nil, nil, true)
	out := strings.Join(lines, "\n")

	if !strings.Contains(out, "[Alarm pending reschedule]") {
		t.Fatalf("expected pending reschedule header, got %q", out)
	}
	if !strings.Contains(out, "not rescheduled") {
		t.Fatalf("expected reschedule reminder, got %q", out)
	}
}

func TestFormatAlarmAlreadyFiredOneShot(t *testing.T) {
	now := time.Now()
	alarm := Alarm{
		ID:          "alarm-4",
		Goal:        "send email",
		NextFireAt:  sql.NullTime{Valid: true, Time: now.Add(-time.Hour)},
		LastFiredAt: sql.NullTime{Valid: true, Time: now},
	}

	lines := formatAlarm(alarm, "", nil, nil, nil, true)
	out := strings.Join(lines, "\n")

	if !strings.Contains(out, "[Alarm pending reschedule]") {
		t.Fatalf("expected pending header, got %q", out)
	}
	if !strings.Contains(out, "cancel_alarm") {
		t.Fatalf("expected cancel instruction, got %q", out)
	}
}
