package shell

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

func TestHandleRunValidatesRequiredFields(t *testing.T) {
	out, err := handleRun(context.Background(), shellArgs{NextCheckAt: "+1m"}, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty command") {
		t.Fatalf("expected empty command validation, got %q", out)
	}

	out, err = handleRun(context.Background(), shellArgs{Command: "echo hello"}, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "next_check_at required for run") {
		t.Fatalf("expected next_check_at validation, got %q", out)
	}
}

func TestHandleReadMissingSession(t *testing.T) {
	requireTmux(t)

	id := "test-read-missing"

	err := newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	err = killSession(id)
	if err != nil {
		t.Fatalf("killSession: %v", err)
	}

	result, err := handleInteract(shellArgs{SessionID: id, MaxWait: 2})
	if err != nil {
		t.Fatalf("handleInteract: %v", err)
	}

	if strings.Contains(result, `"status":"running"`) {
		t.Fatalf("handleRead reported running for a killed session: %s", result)
	}
}

func TestParseCheckAt(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got time.Time)
	}{
		{
			name:  "rfc3339",
			input: "2026-03-02T06:00:00Z",
			check: func(t *testing.T, got time.Time) {
				want := time.Date(2026, 3, 2, 6, 0, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Fatalf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "relative seconds",
			input: "+30s",
			check: func(t *testing.T, got time.Time) {
				diff := got.Sub(now)
				if diff < 29*time.Second || diff > 31*time.Second {
					t.Fatalf("expected ~30s from now, got %v", diff)
				}
			},
		},
		{
			name:  "relative minutes",
			input: "+5m",
			check: func(t *testing.T, got time.Time) {
				diff := got.Sub(now)
				if diff < 4*time.Minute || diff > 6*time.Minute {
					t.Fatalf("expected ~5m from now, got %v", diff)
				}
			},
		},
		{
			name:  "relative hours",
			input: "+1h",
			check: func(t *testing.T, got time.Time) {
				diff := got.Sub(now)
				if diff < 59*time.Minute || diff > 61*time.Minute {
					t.Fatalf("expected ~1h from now, got %v", diff)
				}
			},
		},
		{
			name:  "relative days",
			input: "+1d",
			check: func(t *testing.T, got time.Time) {
				diff := got.Sub(now)
				if diff < 23*time.Hour || diff > 25*time.Hour {
					t.Fatalf("expected ~24h from now, got %v", diff)
				}
			},
		},
		{name: "garbage", input: "not-a-time", wantErr: true},
		{name: "invalid relative", input: "+abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCheckAt(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %v", tt.input, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}

			tt.check(t, got)
		})
	}
}

func TestShortIDCollision(t *testing.T) {
	seen := make(map[string]bool)
	collisions := 0

	for i := 0; i < 20; i++ {
		sid := id.Short(4)
		if seen[sid] {
			collisions++
		}
		seen[sid] = true
	}

	if collisions > 0 {
		t.Fatalf("got %d collisions in 20 sequential IDs", collisions)
	}
}
