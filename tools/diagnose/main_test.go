package main

import (
	"testing"
	"time"
)

func TestShortID(t *testing.T) {
	got := shortID("019d0e0c-79e9-7adb-be0f-5ceb8519bcfd")
	if got != "5ceb8519bcfd" {
		t.Errorf("shortID = %q, want %q", got, "5ceb8519bcfd")
	}
}

func TestShortIDShort(t *testing.T) {
	got := shortID("abc")
	if got != "abc" {
		t.Errorf("shortID = %q, want %q", got, "abc")
	}
}

func TestTruncate(t *testing.T) {
	got := truncate("hello world", 5)
	if got != "hello..." {
		t.Errorf("truncate = %q, want %q", got, "hello...")
	}
}

func TestTruncateNoOp(t *testing.T) {
	got := truncate("hi", 10)
	if got != "hi" {
		t.Errorf("truncate = %q, want %q", got, "hi")
	}
}

func TestParseLogTime(t *testing.T) {
	line := `time=2026-03-20T18:39:47.073-07:00 level=INFO msg="test"`
	ts, ok := parseLogTime(line)
	if !ok {
		t.Fatal("parseLogTime returned false")
	}

	want, _ := time.Parse("2006-01-02T15:04:05.000-07:00", "2026-03-20T18:39:47.073-07:00")
	if !ts.Equal(want) {
		t.Errorf("parseLogTime = %v, want %v", ts, want)
	}
}

func TestParseLogTimeNoPrefix(t *testing.T) {
	_, ok := parseLogTime("no time here")
	if ok {
		t.Error("parseLogTime should return false for line without time=")
	}
}

func TestClassifyNoActivations(t *testing.T) {
	msg := messageRow{Body: "test"}
	d := classify(msg, nil)
	if d.Category != "NO_ACTIVATION" {
		t.Errorf("category = %q, want NO_ACTIVATION", d.Category)
	}
}

func TestClassifyActivationFailed(t *testing.T) {
	msg := messageRow{Body: "test"}
	acts := []activationRow{{
		ID:    "act-1",
		Error: "something broke",
	}}
	d := classify(msg, acts)
	if d.Category != "ACTIVATION_FAILED" {
		t.Errorf("category = %q, want ACTIVATION_FAILED", d.Category)
	}
}

func TestClassifyToolFailed(t *testing.T) {
	msg := messageRow{Body: "test"}
	acts := []activationRow{{
		ID: "act-1",
		Rounds: []roundInfo{{
			Round: 0,
			ToolCalls: []toolCallInfo{{
				Name:  "task_spawn",
				Error: 1,
			}},
		}},
	}}
	d := classify(msg, acts)
	if d.Category != "TOOL_FAILED" {
		t.Errorf("category = %q, want TOOL_FAILED", d.Category)
	}
}

func TestClassifySurfacedNotActed(t *testing.T) {
	msg := messageRow{Body: "test"}
	acts := []activationRow{{
		ID: "act-1",
		Rounds: []roundInfo{{
			Round:        1,
			MessageInNew: true,
		}},
	}}
	d := classify(msg, acts)
	if d.Category != "SURFACED_NOT_ACTED" {
		t.Errorf("category = %q, want SURFACED_NOT_ACTED", d.Category)
	}
}

func TestClassifySurfacedHandled(t *testing.T) {
	msg := messageRow{Body: "test"}
	acts := []activationRow{{
		ID: "act-1",
		Rounds: []roundInfo{{
			Round:        0,
			MessageInNew: true,
			ToolCalls: []toolCallInfo{{
				Name: "message_send",
			}},
		}},
	}}
	d := classify(msg, acts)
	if d.Category != "SURFACED_HANDLED" {
		t.Errorf("category = %q, want SURFACED_HANDLED", d.Category)
	}
}

func TestClassifySurfacedHandledReactOnly(t *testing.T) {
	msg := messageRow{Body: "test"}
	acts := []activationRow{{
		ID: "act-1",
		Rounds: []roundInfo{{
			Round:        0,
			MessageInNew: true,
			ToolCalls: []toolCallInfo{{
				Name: "message_react",
			}},
		}},
	}}
	d := classify(msg, acts)
	if d.Category != "SURFACED_HANDLED" {
		t.Errorf("category = %q, want SURFACED_HANDLED", d.Category)
	}
	if d.Summary == "" || !contains(d.Summary, "react only") {
		t.Errorf("summary should mention react only, got %q", d.Summary)
	}
}

func TestClassifyNotSurfaced(t *testing.T) {
	msg := messageRow{Body: "test"}
	acts := []activationRow{{
		ID: "act-1",
		Rounds: []roundInfo{{
			Round:          0,
			MessagePresent: false,
			ToolCalls: []toolCallInfo{{
				Name: "message_noop",
			}},
		}},
	}}
	d := classify(msg, acts)
	if d.Category != "NOT_SURFACED" {
		t.Errorf("category = %q, want NOT_SURFACED", d.Category)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
