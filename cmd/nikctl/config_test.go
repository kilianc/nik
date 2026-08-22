package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The shape of a real capsule's config, trimmed to the parts that matter.
const capsuleConfig = `# models
models:
  main:
    model: gpt-5.6-sol
    backend: api               # api | subscription

# shell sandbox
shell:
  docker_image: nik-shell

# gateway
gateway:
  url: wss://gateway-dev.hellonik.com/v1/agent
`

func write(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// read parses a config back the way nikd does, so a test asserts on the
// meaning of the file rather than on its text.
func read(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("the config no longer parses: %v\n%s", err, data)
	}
	return out
}

// This is the bug that took a capsule down on 2026-08-22, written as a test.
//
// nik-saas set the sandbox's endpoints with `sed -i '/^shell:/a\  env:'` and
// then inserted under `^  env:`. The insert landed above shell.docker_image
// and the second insert pushed it inside the new map, so the file stayed valid
// YAML while meaning something else: no shell.docker_image, so the shell tool
// ran locally instead of in a container, so nikd looked for tmux in a capsule
// that has no reason to carry one, and exited.
func TestSettingShellEnvLeavesDockerImageWhereItWas(t *testing.T) {
	path := write(t, capsuleConfig)

	for _, kv := range [][2]string{
		{"shell.env.EXA_BASE_URL", "https://exa-dev.hellonik.com"},
		{"shell.env.X_BASE_URL", "https://x-dev.hellonik.com"},
	} {
		if err := configSet(path, kv[0], kv[1]); err != nil {
			t.Fatalf("set %s: %v", kv[0], err)
		}
	}

	cfg := read(t, path)
	shell, ok := cfg["shell"].(map[string]any)
	if !ok {
		t.Fatalf("shell is not a map: %#v", cfg["shell"])
	}
	if got := shell["docker_image"]; got != "nik-shell" {
		t.Errorf("shell.docker_image is %#v, want \"nik-shell\" — the sandbox will run locally", got)
	}
	env, ok := shell["env"].(map[string]any)
	if !ok {
		t.Fatalf("shell.env is not a map: %#v", shell["env"])
	}
	if got := env["EXA_BASE_URL"]; got != "https://exa-dev.hellonik.com" {
		t.Errorf("shell.env.EXA_BASE_URL is %#v", got)
	}
	if got := env["X_BASE_URL"]; got != "https://x-dev.hellonik.com" {
		t.Errorf("shell.env.X_BASE_URL is %#v", got)
	}
	if _, leaked := env["docker_image"]; leaked {
		t.Error("docker_image ended up inside shell.env, which is exactly the bug")
	}
}

// Setting a key that is already there replaces the value and nothing else.
func TestSettingAnExistingKeyReplacesOnlyIt(t *testing.T) {
	path := write(t, capsuleConfig)

	if err := configSet(path, "models.main.base_url", "https://openai-dev.hellonik.com/v1"); err != nil {
		t.Fatal(err)
	}
	if err := configSet(path, "models.main.backend", "api"); err != nil {
		t.Fatal(err)
	}

	cfg := read(t, path)
	main := cfg["models"].(map[string]any)["main"].(map[string]any)
	if got := main["base_url"]; got != "https://openai-dev.hellonik.com/v1" {
		t.Errorf("base_url is %#v", got)
	}
	if got := main["model"]; got != "gpt-5.6-sol" {
		t.Errorf("model became %#v — a sibling was disturbed", got)
	}
	if got := main["backend"]; got != "api" {
		t.Errorf("backend is %#v", got)
	}
}

// The file is read by people, so it comes back looking like itself.
func TestSettingAKeyKeepsCommentsAndOrder(t *testing.T) {
	path := write(t, capsuleConfig)
	if err := configSet(path, "models.main.base_url", "https://openai-dev.hellonik.com/v1"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	for _, want := range []string{"# models", "# shell sandbox", "# gateway", "api | subscription"} {
		if !strings.Contains(body, want) {
			t.Errorf("comment %q was dropped", want)
		}
	}
	if strings.Index(body, "models:") > strings.Index(body, "shell:") {
		t.Error("top-level keys were reordered")
	}
}

// An empty map — `env:` with nothing under it — is a place to put things,
// not a value somebody chose.
func TestAnEmptyMapCanBeFilled(t *testing.T) {
	path := write(t, "shell:\n  env:\n  docker_image: nik-shell\n")
	if err := configSet(path, "shell.env.EXA_BASE_URL", "https://exa.example"); err != nil {
		t.Fatal(err)
	}
	shell := read(t, path)["shell"].(map[string]any)
	if got := shell["docker_image"]; got != "nik-shell" {
		t.Errorf("docker_image is %#v", got)
	}
	if got := shell["env"].(map[string]any)["EXA_BASE_URL"]; got != "https://exa.example" {
		t.Errorf("EXA_BASE_URL is %#v", got)
	}
}

// Refused rather than replaced. A path segment holding a value means the
// config is not shaped the way the caller believes, and overwriting it to
// satisfy that belief destroys whatever was actually there.
func TestSettingUnderAValueIsRefused(t *testing.T) {
	path := write(t, "shell: nik-shell\n")
	err := configSet(path, "shell.env.EXA_BASE_URL", "https://exa.example")
	if err == nil {
		t.Fatal("overwrote a scalar with a map")
	}
	if !strings.Contains(err.Error(), "value") {
		t.Errorf("error does not say what is in the way: %v", err)
	}
}

// A quoted empty string is still a scalar, and replacing it must not leave
// the quotes wrapped around the new value.
func TestReplacingAQuotedEmptyValue(t *testing.T) {
	path := write(t, "models:\n  main:\n    base_url: \"\"\n")
	if err := configSet(path, "models.main.base_url", "https://openai-dev.hellonik.com/v1"); err != nil {
		t.Fatal(err)
	}
	main := read(t, path)["models"].(map[string]any)["main"].(map[string]any)
	if got := main["base_url"]; got != "https://openai-dev.hellonik.com/v1" {
		t.Errorf("base_url is %#v — quoting leaked into the value", got)
	}
}

// Setting the same key twice leaves one key, because an installer runs again.
func TestSettingIsIdempotent(t *testing.T) {
	path := write(t, capsuleConfig)
	for i := 0; i < 3; i++ {
		if err := configSet(path, "shell.env.EXA_BASE_URL", "https://exa.example"); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	if n := strings.Count(string(data), "EXA_BASE_URL"); n != 1 {
		t.Errorf("EXA_BASE_URL appears %d times, want 1", n)
	}
}

func TestGetReadsBackWhatSetWrote(t *testing.T) {
	path := write(t, capsuleConfig)
	if err := configSet(path, "models.main.base_url", "https://openai-dev.hellonik.com/v1"); err != nil {
		t.Fatal(err)
	}
	got, err := configGet(path, "models.main.base_url")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://openai-dev.hellonik.com/v1" {
		t.Errorf("get returned %q", got)
	}
	if _, err := configGet(path, "models.main.nothing"); err == nil {
		t.Error("get invented a value for a key that is not there")
	}
}
