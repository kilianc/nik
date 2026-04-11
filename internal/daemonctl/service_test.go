package daemonctl

import (
	"runtime"
	"testing"
)

func TestIsInstalledReturnsBool(t *testing.T) {
	got := IsInstalled()
	if got && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Errorf("unexpected IsInstalled=true on %s", runtime.GOOS)
	}
}

func TestIsRunningDoesNotPanic(t *testing.T) {
	_, err := IsRunning()
	if err != nil && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Logf("IsRunning returned error on %s: %v", runtime.GOOS, err)
	}
}
