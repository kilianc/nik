package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTTSInstructionsPathPrefersWorkspaceOverride(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "builtin")
	err := os.MkdirAll(promptsDir, 0o755)
	if err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	err = os.WriteFile(filepath.Join(promptsDir, "tts-00.md"), []byte("builtin"), 0o644)
	if err != nil {
		t.Fatalf("write builtin tts: %v", err)
	}

	c := Config{Home: dir, PromptsDirValue: promptsDir}

	got := c.TTSInstructionsPath()
	if got != filepath.Join(promptsDir, "tts-00.md") {
		t.Fatalf("expected builtin path, got %q", got)
	}

	wsPrompts := filepath.Join(dir, "prompts")
	err = os.MkdirAll(wsPrompts, 0o755)
	if err != nil {
		t.Fatalf("mkdir workspace prompts: %v", err)
	}
	err = os.WriteFile(filepath.Join(wsPrompts, "tts-00.md"), []byte("override"), 0o644)
	if err != nil {
		t.Fatalf("write workspace tts: %v", err)
	}

	got = c.TTSInstructionsPath()
	if got != filepath.Join(wsPrompts, "tts-00.md") {
		t.Fatalf("expected workspace override path %q, got %q", filepath.Join(wsPrompts, "tts-00.md"), got)
	}
}

func TestMediaPathJoinsHomeAndMedia(t *testing.T) {
	c := Config{Home: "/tmp/nik"}

	if got := c.MediaPath(); got != filepath.Join("/tmp/nik", "media") {
		t.Fatalf("expected media path under home, got %q", got)
	}
}

func TestTmpPathUsesWorkspaceTmp(t *testing.T) {
	c := Config{Home: "/tmp/nik"}

	if got := c.TmpPath(); got != filepath.Join("/tmp/nik", "tmp") {
		t.Fatalf("expected tmp path, got %q", got)
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
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-4
    reasoning_effort: low
    verbosity: medium
timezone: UTC
max_history: 50
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Models.Main.Model != "gpt-4" {
		t.Fatalf("expected model gpt-4, got %q", cfg.Models.Main.Model)
	}

	time.Sleep(50 * time.Millisecond)
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-5
    reasoning_effort: high
    verbosity: low
timezone: America/Chicago
max_history: 200
location: Chicago
`)

	reloaded, err := cfg.ReloadIfChanged()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded {
		t.Fatal("expected reload to return true")
	}

	if cfg.Models.Main.Model != "gpt-5" {
		t.Fatalf("expected model gpt-5 after reload, got %q", cfg.Models.Main.Model)
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
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-4
`)

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
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-4
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	modelPtr := &cfg.Models.Main.Model

	time.Sleep(50 * time.Millisecond)
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-5
`)

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
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-4
privileged_conversation_ids:
  priv: priv1
allow_conversation_ids:
  conv: conv1
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(cfg.AllowConversationIDs) != 2 {
		t.Fatalf("expected 2 allow IDs after load, got %d", len(cfg.AllowConversationIDs))
	}

	time.Sleep(50 * time.Millisecond)
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-4
privileged_conversation_ids:
  priv: priv1
  priv2: priv2
allow_conversation_ids:
  conv: conv1
`)

	_, err = cfg.ReloadIfChanged()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if len(cfg.AllowConversationIDs) != 3 {
		t.Fatalf("expected 3 allow IDs after reload (conv1 + priv1 + priv2), got %d: %v", len(cfg.AllowConversationIDs), cfg.AllowConversationIDs)
	}
}

func TestLoadRejectsMissingMainModel(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: ""
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing models.main.model")
	}
}

func TestLoadRejectsEnabledCriticWithoutModel(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-5
  critic:
    enabled: true
    model: ""
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when critic is enabled without model")
	}
}

func TestLoadRejectsInvalidPurposeSettings(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-5
    reasoning_effort: turbo
`)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid models.main.reasoning_effort")
	}
}

func TestIsAllowed(t *testing.T) {
	cfg := Config{
		AllowConversationIDs: map[string]string{"owner": "conv-1", "friend": "conv-2"},
	}

	if !cfg.IsAllowed("conv-1") {
		t.Fatal("expected conv-1 to be allowed")
	}
	if !cfg.IsAllowed("conv-2") {
		t.Fatal("expected conv-2 to be allowed")
	}
	if cfg.IsAllowed("conv-999") {
		t.Fatal("expected conv-999 to not be allowed")
	}
	if cfg.IsAllowed("") {
		t.Fatal("expected empty string to not be allowed")
	}
}

func TestLoadIgnoresLegacyExaAPIKey(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `
openai_key: sk-test
exa_api_key: exa-old-key
models:
  main:
    model: gpt-5
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Models.Main.Model != "gpt-5" {
		t.Fatalf("expected models.main.model gpt-5, got %q", cfg.Models.Main.Model)
	}
}
