package shell

import (
	"context"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func TestRunCommandLocal(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		stdin string
		want  string
	}{
		{"echo", "echo hello", "", "hello"},
		{"stdin passthrough", "cat", "input-data", "input-data"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			svc := &Service{cfg: &config.Config{Home: dir}}

			stdout, stderr, err := svc.RunCommand(context.Background(), tt.cmd, tt.stdin)
			if err != nil {
				t.Fatalf("run command: %v (stderr: %s)", err, stderr)
			}

			if got := strings.TrimSpace(stdout); got != tt.want {
				t.Errorf("expected stdout %q, got %q", tt.want, got)
			}
		})
	}
}
