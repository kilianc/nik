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
		{"empty goal", `{"origin_contact_id":"contact-1","goal":"","fire_at":"2026-01-01T00:00:00Z"}`, "empty goal"},
		{"empty origin_contact_id", `{"origin_contact_id":"","goal":"check in","fire_at":"2026-01-01T00:00:00Z"}`, "empty origin_contact_id"},
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
