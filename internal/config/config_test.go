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

func TestPathGetters(t *testing.T) {
	c := Config{Home: "/tmp/nik"}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"MediaPath", c.MediaPath(), filepath.Join("/tmp/nik", "media")},
		{"TmpPath", c.TmpPath(), filepath.Join("/tmp/nik", "tmp")},
		{"ErrLogPath", c.ErrLogPath(), filepath.Join("/tmp/nik", "nik.err.log")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, tt.got)
			}
		})
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

func TestLoadValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			"missing main model",
			"openai_key: sk-test\nmodels:\n  main:\n    model: \"\"\n",
			true,
		},
		{
			"critic enabled without model",
			"openai_key: sk-test\nmodels:\n  main:\n    model: gpt-5\n  critic:\n    enabled: true\n    model: \"\"\n",
			true,
		},
		{
			"invalid main reasoning_effort",
			"openai_key: sk-test\nmodels:\n  main:\n    model: gpt-5\n    reasoning_effort: turbo\n",
			true,
		},
		{
			"invalid task reasoning_effort",
			"openai_key: sk-test\nmodels:\n  main:\n    model: gpt-5\n  task:\n    reasoning_effort: turbo\n",
			true,
		},
		{
			"empty backend is valid",
			"openai_key: sk-test\nmodels:\n  main:\n    model: gpt-5\n    backend: \"\"\n",
			false,
		},
		{
			"api backend is valid",
			"openai_key: sk-test\nmodels:\n  main:\n    model: gpt-5\n    backend: api\n",
			false,
		},
		{
			"subscription backend is valid",
			"models:\n  main:\n    model: gpt-5\n    backend: subscription\n",
			false,
		},
		{
			"invalid backend rejected",
			"openai_key: sk-test\nmodels:\n  main:\n    model: gpt-5\n    backend: cloud\n",
			true,
		},
		{
			"invalid task backend rejected",
			"openai_key: sk-test\nmodels:\n  main:\n    model: gpt-5\n  task:\n    backend: cloud\n",
			true,
		},
		{
			"invalid critic backend rejected",
			"openai_key: sk-test\nmodels:\n  main:\n    model: gpt-5\n  critic:\n    backend: cloud\n",
			true,
		},
		{
			"subscription satisfies auth requirement",
			"models:\n  main:\n    model: gpt-5\n    backend: subscription\n",
			false,
		},
		{
			"no key and no subscription fails",
			"models:\n  main:\n    model: gpt-5\n",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestConfig(t, dir, tt.config)

			_, err := Load(dir)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadAcceptsTaskModel(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-5
  task:
    model: gpt-4.1-mini
    reasoning_effort: low
    verbosity: medium
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Models.Task.Model != "gpt-4.1-mini" {
		t.Fatalf("expected models.task.model gpt-4.1-mini, got %q", cfg.Models.Task.Model)
	}
	if cfg.Models.Task.ReasoningEffort != "low" {
		t.Fatalf("expected models.task.reasoning_effort low, got %q", cfg.Models.Task.ReasoningEffort)
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

func TestIsSubscription(t *testing.T) {
	m := ModelConfig{Backend: "subscription"}
	if !m.IsSubscription() {
		t.Fatal("expected IsSubscription true")
	}

	m.Backend = "api"
	if m.IsSubscription() {
		t.Fatal("expected IsSubscription false for api")
	}

	m.Backend = ""
	if m.IsSubscription() {
		t.Fatal("expected IsSubscription false for empty")
	}

	c := CriticConfig{Backend: "subscription"}
	if !c.IsSubscription() {
		t.Fatal("expected CriticConfig.IsSubscription true")
	}

	c.Backend = ""
	if c.IsSubscription() {
		t.Fatal("expected CriticConfig.IsSubscription false for empty")
	}
}

func TestAnySubscription(t *testing.T) {
	m := ModelsConfig{}
	if m.AnySubscription() {
		t.Fatal("expected false when no backend set")
	}

	m.Task.Backend = "subscription"
	if !m.AnySubscription() {
		t.Fatal("expected true when task is subscription")
	}

	m = ModelsConfig{}
	m.Critic.Backend = "subscription"
	if !m.AnySubscription() {
		t.Fatal("expected true when critic is subscription")
	}
}

func TestTaskConfigDefaults(t *testing.T) {
	var tc TaskConfig

	if got := tc.MaxRoundsOrDefault(); got != 200 {
		t.Fatalf("expected default max_rounds 200, got %d", got)
	}
	if got := tc.TimeoutOrDefault(); got != 60*time.Minute {
		t.Fatalf("expected default timeout 60m, got %v", got)
	}

	tc.MaxRounds = 150
	tc.Timeout = 90 * time.Minute

	if got := tc.MaxRoundsOrDefault(); got != 150 {
		t.Fatalf("expected max_rounds 150, got %d", got)
	}
	if got := tc.TimeoutOrDefault(); got != 90*time.Minute {
		t.Fatalf("expected timeout 90m, got %v", got)
	}
}

func TestLoadParsesTaskConfig(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-5
task:
  max_rounds: 250
  timeout: 45m
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Task.MaxRounds != 250 {
		t.Fatalf("expected task.max_rounds 250, got %d", cfg.Task.MaxRounds)
	}
	if cfg.Task.Timeout != 45*time.Minute {
		t.Fatalf("expected task.timeout 45m, got %v", cfg.Task.Timeout)
	}
}

func TestLoadOmittedTaskConfigUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `
openai_key: sk-test
models:
  main:
    model: gpt-5
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got := cfg.Task.MaxRoundsOrDefault(); got != 200 {
		t.Fatalf("expected default max_rounds 200, got %d", got)
	}
	if got := cfg.Task.TimeoutOrDefault(); got != 60*time.Minute {
		t.Fatalf("expected default timeout 60m, got %v", got)
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
