package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOverrideWins(t *testing.T) {
	t.Setenv("NIK_HOME", filepath.Join(t.TempDir(), "from-env"))
	want := filepath.Join(t.TempDir(), "explicit")

	got, err := Resolve(want)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveFallsBackToEnv(t *testing.T) {
	want := filepath.Join(t.TempDir(), "from-env")
	t.Setenv("NIK_HOME", want)

	got, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "nik")

	got, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat %q: %v", got, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", got)
	}
}

// A relative --home has to come back absolute: nikd writes it into a service
// file, and launchd starts from a working directory nobody chose.
func TestResolveReturnsAbsolute(t *testing.T) {
	wd := t.TempDir()
	t.Chdir(wd)

	got, err := Resolve("relative-home")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("got %q, want an absolute path", got)
	}
}
