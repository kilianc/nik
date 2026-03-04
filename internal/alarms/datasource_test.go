package alarms

import (
	"database/sql"
	"strings"
	"testing"
)

func TestFormatAlarmFreshOneShot(t *testing.T) {
	alarm := Alarm{
		ID:              "alarm-1",
		Goal:            "send follow-up",
		OriginContactID: sql.NullString{Valid: true, String: "contact-1"},
	}

	lines := formatAlarm(alarm, "kevin", nil, nil, nil)
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

	lines := formatAlarm(alarm, "", nil, nil, nil)
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

func TestFormatAlarmWithSource(t *testing.T) {
	alarm := Alarm{
		ID:     "alarm-src",
		Goal:   "check in with CT",
		Source: sql.NullString{Valid: true, String: "message"},
	}

	lines := formatAlarm(alarm, "", nil, nil, nil)
	out := strings.Join(lines, "\n")

	if !strings.Contains(out, "Created from: message") {
		t.Fatalf("expected source line, got %q", out)
	}
}

func TestFormatAlarmWithoutSource(t *testing.T) {
	alarm := Alarm{
		ID:   "alarm-nosrc",
		Goal: "do something",
	}

	lines := formatAlarm(alarm, "", nil, nil, nil)
	out := strings.Join(lines, "\n")

	if strings.Contains(out, "Created from:") {
		t.Fatalf("expected no source line for alarm without source, got %q", out)
	}
}
