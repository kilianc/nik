package shell

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func TestRunCommandLocal(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cfg: &config.Config{Home: dir}}

	stdout, stderr, err := svc.RunCommand(context.Background(), "echo hello", "")
	if err != nil {
		t.Fatalf("run command: %v (stderr: %s)", err, stderr)
	}

	if got := strings.TrimSpace(stdout); got != "hello" {
		t.Fatalf("expected stdout %q, got %q", "hello", got)
	}
}

func TestRunCommandLocalPassesStdin(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cfg: &config.Config{Home: dir}}

	stdout, _, err := svc.RunCommand(context.Background(), "cat", "input-data")
	if err != nil {
		t.Fatalf("run command: %v", err)
	}

	if got := strings.TrimSpace(stdout); got != "input-data" {
		t.Fatalf("expected stdout %q, got %q", "input-data", got)
	}
}
