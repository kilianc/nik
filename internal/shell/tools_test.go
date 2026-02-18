package shell

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHandleRunValidatesRequiredFields(t *testing.T) {
	out, err := handleRun(context.Background(), shellArgs{NextCheckAt: "+1m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty command") {
		t.Fatalf("expected empty command validation, got %q", out)
	}

	out, err = handleRun(context.Background(), shellArgs{Command: "echo hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "next_check_at required for run") {
		t.Fatalf("expected next_check_at validation, got %q", out)
	}
}

func TestParseRelativeDurationDays(t *testing.T) {
	d, err := parseRelativeDuration("2d")
	if err != nil {
		t.Fatalf("parse relative duration: %v", err)
	}
	if d != 48*time.Hour {
		t.Fatalf("expected 48h, got %v", d)
	}
}
