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

func TestAlarmHandlerValidatesGoal(t *testing.T) {
	handler := alarmHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"origin_contact_id":"contact-1","goal":"","fire_at":"2026-01-01T00:00:00Z"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty goal") {
		t.Fatalf("expected empty goal validation, got %q", out)
	}
}

func TestAlarmHandlerValidatesOriginContactID(t *testing.T) {
	handler := alarmHandler(&Service{})

	out, err := handler(context.Background(), llm.ToolCall{
		Arguments: `{"origin_contact_id":"","goal":"check in","fire_at":"2026-01-01T00:00:00Z"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty origin_contact_id") {
		t.Fatalf("expected empty origin_contact_id validation, got %q", out)
	}
}
