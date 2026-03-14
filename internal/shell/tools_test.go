package shell

import (
	"context"
	"strings"
	"testing"
)

func TestHandleRunValidatesRequiredFields(t *testing.T) {
	svc := &Service{home: t.TempDir()}
	out, err := svc.handleRun(context.Background(), shellArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty command") {
		t.Fatalf("expected empty command validation, got %q", out)
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

	svc := &Service{}
	result, err := svc.handleInteract(context.Background(), shellArgs{SessionID: id, MaxWait: 2})
	if err != nil {
		t.Fatalf("handleInteract: %v", err)
	}

	if strings.Contains(result, `"status":"running"`) {
		t.Fatalf("handleRead reported running for a killed session: %s", result)
	}
}
