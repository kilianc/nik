package task

import (
	"testing"

	"github.com/kciuffolo/nik/internal/db"
)

func TestExtractSkillNames(t *testing.T) {
	calls := []db.ToolCallInfo{
		{Name: "load_skill", Input: `{"action":"load","name":"web","reason":"fetch tweet"}`},
		{Name: "shell", Input: `{"action":"run","command":"ls"}`},
		{Name: "load_skill", Input: `{"action":"list","name":""}`},
		{Name: "load_skill", Input: `{"action":"load","name":"journal","reason":"write entry"}`},
	}

	got := extractSkillNames(calls)
	if got != "web, journal" {
		t.Fatalf("expected 'web, journal', got %q", got)
	}
}

func TestExtractSkillNamesEmpty(t *testing.T) {
	got := extractSkillNames(nil)
	if got != "(none)" {
		t.Fatalf("expected '(none)', got %q", got)
	}
}

func TestFormatToolCalls(t *testing.T) {
	calls := []db.ToolCallInfo{
		{Name: "shell", DurationMS: 120, Error: false},
		{Name: "db_query", DurationMS: 50, Error: true, Output: "no such table: foo"},
	}

	got := formatToolCalls(calls)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestFormatToolCallsEmpty(t *testing.T) {
	got := formatToolCalls(nil)
	if got != "(no tool calls recorded)" {
		t.Fatalf("expected placeholder, got %q", got)
	}
}
