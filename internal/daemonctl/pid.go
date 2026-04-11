package daemonctl

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func pidPath(home string) string {
	return filepath.Join(home, "nik.pid")
}

func WritePID(home string) error {
	return os.WriteFile(pidPath(home), []byte(strconv.Itoa(os.Getpid())), 0o644)
}

func RemovePID(home string) {
	os.Remove(pidPath(home))
}

func ReadPID(home string) (int, bool) {
	data, err := os.ReadFile(pidPath(home))
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}

	err = syscall.Kill(pid, 0)
	if err != nil {
		return 0, false
	}

	return pid, true
}

func SignalDaemon(home string, sig os.Signal) error {
	pid, alive := ReadPID(home)
	if !alive {
		return fmt.Errorf("daemon not running")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	return proc.Signal(sig)
}
