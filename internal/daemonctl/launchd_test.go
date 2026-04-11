package daemonctl

import (
	"runtime"
	"testing"
)

func TestLaunchdPlistPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd only on darwin")
	}

	path, err := launchdPlistPath()
	if err != nil {
		t.Fatalf("launchdPlistPath: %v", err)
	}

	if path == "" {
		t.Fatal("expected non-empty plist path")
	}
}
