package daemonctl

import (
	"runtime"
	"testing"
)

func TestSystemdUnitPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd only on linux")
	}

	path, err := systemdUnitPath()
	if err != nil {
		t.Fatalf("systemdUnitPath: %v", err)
	}

	if path == "" {
		t.Fatal("expected non-empty unit path")
	}
}
