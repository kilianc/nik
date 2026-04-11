package daemonctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadPID(t *testing.T) {
	home := t.TempDir()

	err := WritePID(home)
	if err != nil {
		t.Fatalf("WritePID: %v", err)
	}

	pid, alive := ReadPID(home)
	if !alive {
		t.Fatal("expected daemon to be alive")
	}
	if pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", pid, os.Getpid())
	}
}

func TestReadPIDMissingFile(t *testing.T) {
	home := t.TempDir()

	_, alive := ReadPID(home)
	if alive {
		t.Fatal("expected not alive for missing PID file")
	}
}

func TestReadPIDStaleProcess(t *testing.T) {
	home := t.TempDir()
	os.WriteFile(filepath.Join(home, "nik.pid"), []byte("999999999"), 0o644)

	_, alive := ReadPID(home)
	if alive {
		t.Fatal("expected not alive for stale PID")
	}
}

func TestRemovePID(t *testing.T) {
	home := t.TempDir()

	err := WritePID(home)
	if err != nil {
		t.Fatalf("WritePID: %v", err)
	}

	RemovePID(home)

	_, alive := ReadPID(home)
	if alive {
		t.Fatal("expected not alive after RemovePID")
	}
}

func TestSignalDaemonNotRunning(t *testing.T) {
	home := t.TempDir()

	err := SignalDaemon(home, os.Interrupt)
	if err == nil {
		t.Fatal("expected error for no daemon")
	}
}
