package shell

import "testing"

func TestFormatDurationHandlesMissingAndInvalidValues(t *testing.T) {
	if got := formatDuration(""); got != "unknown" {
		t.Fatalf("expected unknown for empty timestamp, got %q", got)
	}

	if got := formatDuration("not-a-time"); got != "unknown" {
		t.Fatalf("expected unknown for invalid timestamp, got %q", got)
	}
}

func TestFormatConversationContextReturnsNilWithoutMessages(t *testing.T) {
	lines := formatConversationContext(nil, nil)
	if lines != nil {
		t.Fatalf("expected nil lines for empty message context, got %v", lines)
	}
}
