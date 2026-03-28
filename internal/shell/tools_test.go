package shell

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func TestHandleRunValidatesRequiredFields(t *testing.T) {
	svc := NewService(&config.Config{Home: t.TempDir()}, nil)
	out, err := svc.handleRun(context.Background(), shellArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty command") {
		t.Fatalf("expected empty command validation, got %q", out)
	}
}

func TestEnsureReadyNoDocker(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	err := svc.EnsureReady()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if svc.container != "" {
		t.Fatalf("expected empty container, got %q", svc.container)
	}
}

func TestHandleReadMissingSession(t *testing.T) {
	requireTmux(t)
	svc := testService(t)

	id := "test-read-missing"

	err := svc.newSession(id, "sleep 60", "")
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	err = svc.killSession(id)
	if err != nil {
		t.Fatalf("killSession: %v", err)
	}

	result, err := svc.handleInteract(context.Background(), shellArgs{SessionID: id, MaxWait: 2})
	if err != nil {
		t.Fatalf("handleInteract: %v", err)
	}

	if strings.Contains(result, `"status":"running"`) {
		t.Fatalf("handleRead reported running for a killed session: %s", result)
	}
}
