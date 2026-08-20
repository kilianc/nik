package daemonctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SiblingBinary answers relative to os.Executable(), which in a test is the
// test binary itself — so the sibling is created next to that.
func testBinDir(t *testing.T) string {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	return filepath.Dir(resolved)
}

func TestSiblingBinaryFound(t *testing.T) {
	dir := testBinDir(t)
	path := filepath.Join(dir, "nikd-sibling-test")

	err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)
	if err != nil {
		t.Fatalf("write sibling: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	got, err := SiblingBinary("nikd-sibling-test")
	if err != nil {
		t.Fatalf("SiblingBinary: %v", err)
	}
	if got != path {
		t.Fatalf("got %q, want %q", got, path)
	}
}

// A half-unpacked install has to say which half is missing — this error text
// is what `nikctl install` prints, so it is part of the contract.
func TestSiblingBinaryMissingNamesIt(t *testing.T) {
	_, err := SiblingBinary("nikd-definitely-not-here")
	if err == nil {
		t.Fatal("want an error for a missing sibling, got nil")
	}
	if !strings.Contains(err.Error(), "nikd-definitely-not-here") {
		t.Fatalf("error does not name the missing binary: %v", err)
	}
}
