package alarms

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/llm"
)

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

func TestParseLocalTime(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name     string
		input    string
		tz       string
		wantErr  string
		wantLoc  string
		wantHour int
	}{
		{"defaults to UTC", "2026-03-15 09:00", "", "", "UTC", 9},
		{"explicit timezone", "2026-03-15 09:00", "Europe/Rome", "", "Europe/Rome", 8},
		{"invalid timezone", "2026-03-15 09:00", "Not/A/Timezone", "invalid timezone", "", 0},
		{"invalid format", "not-a-time", "", "parse", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.parseLocalTime(tt.input, tt.tz)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantLoc != "" && got.Location().String() != tt.wantLoc {
				t.Fatalf("expected location %s, got %s", tt.wantLoc, got.Location())
			}

			utc := got.UTC()
			if utc.Hour() != tt.wantHour {
				t.Fatalf("expected %d:00 UTC, got %s", tt.wantHour, utc.Format("15:04"))
			}
		})
	}
}
