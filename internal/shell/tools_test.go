package shell

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/id"
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

func TestHandleReadMissingSession(t *testing.T) {
	requireTmux(t)

	id := "test-read-missing"

	err := newSession(id, "sleep 60")
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
