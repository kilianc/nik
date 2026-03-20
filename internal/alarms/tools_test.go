package alarms

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestBuildToolsReturnsAlarmTools(t *testing.T) {
	tools := BuildTools(&Service{})
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Def.Name] = true
	}
	for _, want := range []string{"alarm", "update_alarm", "cancel_alarm"} {
		if !names[want] {
			t.Fatalf("expected %q tool", want)
		}
	}
}

func TestAlarmHandlerValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantSub string
	}{
		{"empty goal", `{"origin_contact_id":"contact-1","goal":"","fire_at":"2026-01-01 00:00"}`, "empty goal"},
		{"empty origin_contact_id", `{"origin_contact_id":"","goal":"check in","fire_at":"2026-01-01 00:00"}`, "empty origin_contact_id"},
	}

	handler := alarmHandler(&Service{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := handler(context.Background(), llm.ToolCall{Arguments: tt.args})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tt.wantSub) {
				t.Fatalf("expected %q in output, got %q", tt.wantSub, out)
			}
		})
	}
}

func TestParseLocalTimeDefaultsToUTC(t *testing.T) {
	svc := &Service{}

	got, err := svc.parseLocalTime("2026-03-15 09:00", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Hour() != 9 || got.Minute() != 0 {
		t.Fatalf("expected 09:00, got %s", got.Format("15:04"))
	}
	if got.Location().String() != "UTC" {
		t.Fatalf("expected UTC, got %s", got.Location())
	}
}

func TestParseLocalTimeUsesExplicitTimezone(t *testing.T) {
	svc := &Service{}

	got, err := svc.parseLocalTime("2026-03-15 09:00", "Europe/Rome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Location().String() != "Europe/Rome" {
		t.Fatalf("expected Europe/Rome, got %s", got.Location())
	}

	utc := got.UTC()
	if utc.Hour() != 8 {
		t.Fatalf("expected 08:00 UTC (Rome is UTC+1 in March), got %s", utc.Format("15:04"))
	}
}

func TestParseLocalTimeRejectsInvalidTimezone(t *testing.T) {
	svc := &Service{}

	_, err := svc.parseLocalTime("2026-03-15 09:00", "Not/A/Timezone")
	if err == nil {
		t.Fatalf("expected error for invalid timezone")
	}
	if !strings.Contains(err.Error(), "invalid timezone") {
		t.Fatalf("expected 'invalid timezone' error, got %v", err)
	}
}

func TestParseLocalTimeRejectsInvalidFormat(t *testing.T) {
	svc := &Service{}

	_, err := svc.parseLocalTime("not-a-time", "")
	if err == nil {
		t.Fatalf("expected error for invalid time format")
	}
}
