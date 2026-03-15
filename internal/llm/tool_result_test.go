package llm

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestToolErrorFormatsJSON(t *testing.T) {
	got := ToolError(errors.New("something broke"))

	var parsed map[string]string
	err := json.Unmarshal([]byte(got), &parsed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["error"] != "something broke" {
		t.Errorf("expected 'something broke', got %q", parsed["error"])
	}
}

func TestToolErrorfFormatsArgs(t *testing.T) {
	got := ToolErrorf("missing %s: %d", "item", 42)

	var parsed map[string]string
	err := json.Unmarshal([]byte(got), &parsed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["error"] != "missing item: 42" {
		t.Errorf("expected 'missing item: 42', got %q", parsed["error"])
	}
}

func TestToolResultMarshalsValue(t *testing.T) {
	got := ToolResult(map[string]int{"count": 5})

	var parsed map[string]int
	err := json.Unmarshal([]byte(got), &parsed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["count"] != 5 {
		t.Errorf("expected count=5, got %d", parsed["count"])
	}
}
