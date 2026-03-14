package config

import (
	"os"
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

func writeTestConfig(t *testing.T, dir, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644)
	if err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
}

func TestReloadIfChangedPicksUpNewValues(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, "openai_key: sk-test\nmodel: gpt-4\ntimezone: UTC\nmax_history: 50\n")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Model != "gpt-4" {
		t.Fatalf("expected model gpt-4, got %q", cfg.Model)
	}

	time.Sleep(50 * time.Millisecond)
	writeTestConfig(t, dir, "openai_key: sk-test\nmodel: gpt-5\ntimezone: America/Chicago\nmax_history: 200\nlocation: Chicago\n")

	reloaded, err := cfg.ReloadIfChanged()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded {
		t.Fatal("expected reload to return true")
	}

	if cfg.Model != "gpt-5" {
		t.Fatalf("expected model gpt-5 after reload, got %q", cfg.Model)
	}
	if cfg.Timezone != "America/Chicago" {
		t.Fatalf("expected timezone America/Chicago, got %q", cfg.Timezone)
	}
	if cfg.MaxHistory != 200 {
		t.Fatalf("expected max_history 200, got %d", cfg.MaxHistory)
	}
	if cfg.Location != "Chicago" {
		t.Fatalf("expected location Chicago, got %q", cfg.Location)
	}
}

func TestReloadIfChangedNoOpWhenUnmodified(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, "openai_key: sk-test\nmodel: gpt-4\n")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	reloaded, err := cfg.ReloadIfChanged()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded {
		t.Fatal("expected no reload when file unchanged")
	}
}

func TestReloadIfChangedPreservesPointer(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, "openai_key: sk-test\nmodel: gpt-4\n")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	modelPtr := &cfg.Model

	time.Sleep(50 * time.Millisecond)
	writeTestConfig(t, dir, "openai_key: sk-test\nmodel: gpt-5\n")

	_, err = cfg.ReloadIfChanged()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if *modelPtr != "gpt-5" {
		t.Fatalf("pointer should see new value gpt-5, got %q", *modelPtr)
	}
}

func TestReloadIfChangedMergesPrivilegedIntoAllow(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, "openai_key: sk-test\nprivileged_conversation_ids:\n  priv: priv1\nallow_conversation_ids:\n  conv: conv1\n")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(cfg.AllowConversationIDs) != 2 {
		t.Fatalf("expected 2 allow IDs after load, got %d", len(cfg.AllowConversationIDs))
	}

	time.Sleep(50 * time.Millisecond)
	writeTestConfig(t, dir, "openai_key: sk-test\nprivileged_conversation_ids:\n  priv: priv1\n  priv2: priv2\nallow_conversation_ids:\n  conv: conv1\n")

	_, err = cfg.ReloadIfChanged()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if len(cfg.AllowConversationIDs) != 3 {
		t.Fatalf("expected 3 allow IDs after reload (conv1 + priv1 + priv2), got %d: %v", len(cfg.AllowConversationIDs), cfg.AllowConversationIDs)
	}
}
