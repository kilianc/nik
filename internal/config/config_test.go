package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMediaPathHandlesDefaultRelativeAndAbsolute(t *testing.T) {
	c := Config{Home: "/tmp/nik"}

	if got := c.MediaPath(); got != filepath.Join("/tmp/nik", "media") {
		t.Fatalf("expected default media path, got %q", got)
	}

	c.MediaDirValue = "files"
	if got := c.MediaPath(); got != filepath.Join("/tmp/nik", "files") {
		t.Fatalf("expected relative media path, got %q", got)
	}

	c.MediaDirValue = "/var/tmp/media"
	if got := c.MediaPath(); got != "/var/tmp/media" {
		t.Fatalf("expected absolute media path, got %q", got)
	}
}

func TestTZFallsBackToLocalForInvalidTimezone(t *testing.T) {
	c := Config{Timezone: "Invalid/Timezone"}
	if c.TZ().String() != time.Local.String() {
		t.Fatalf("expected local timezone fallback, got %q", c.TZ().String())
	}
}
