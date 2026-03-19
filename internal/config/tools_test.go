package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigSetRejectsReadOnlyAndUnknownFields(t *testing.T) {
	cfg := &Config{}

	out, err := configSet(cfg, "openai_key", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "read-only") {
		t.Fatalf("expected read-only error, got %q", out)
	}

	out, err = configSet(cfg, "does_not_exist", "value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "unknown field") {
		t.Fatalf("expected unknown field error, got %q", out)
	}
}

func TestAllowlistRemoveGuardsLastEntry(t *testing.T) {
	cfg := &Config{
		AllowConversationIDs:      map[string]string{"owner": "owner-conv"},
		PrivilegedConversationIDs: map[string]string{"owner": "owner-conv"},
	}

	out, err := allowlistRemove(cfg, "owner-conv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "cannot remove last allow list entry") {
		t.Fatalf("expected guard error, got %q", out)
	}
}

func TestConfigSetSupportsPurposeModelFields(t *testing.T) {
	cfg := &Config{
		Home:      t.TempDir(),
		OpenAIKey: "sk-test",
		Models: ModelsConfig{
			Main: ModelConfig{
				Model: "gpt-5",
			},
			Recall: ModelConfig{
				Model: "gpt-4.1-nano",
			},
			Critic: CriticConfig{
				Model: "gpt-4.1-nano",
			},
		},
	}

	out, err := configSet(cfg, "models.main.model", "gpt-5.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("expected ok response, got %q", out)
	}
	if cfg.Models.Main.Model != "gpt-5.4" {
		t.Fatalf("expected models.main.model to update, got %q", cfg.Models.Main.Model)
	}

	out, err = configSet(cfg, "models.critic.enabled", "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("expected ok response, got %q", out)
	}
	if !cfg.Models.Critic.Enabled {
		t.Fatal("expected models.critic.enabled to be true")
	}
}

func TestConfigSetSupportsTaskModelFields(t *testing.T) {
	cfg := &Config{
		Home:      t.TempDir(),
		OpenAIKey: "sk-test",
		Models: ModelsConfig{
			Main: ModelConfig{Model: "gpt-5"},
		},
	}

	out, err := configSet(cfg, "models.task.model", "gpt-4.1-mini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("expected ok response, got %q", out)
	}
	if cfg.Models.Task.Model != "gpt-4.1-mini" {
		t.Fatalf("expected models.task.model gpt-4.1-mini, got %q", cfg.Models.Task.Model)
	}

	out, err = configSet(cfg, "models.task.reasoning_effort", "high")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("expected ok response, got %q", out)
	}
	if cfg.Models.Task.ReasoningEffort != "high" {
		t.Fatalf("expected models.task.reasoning_effort high, got %q", cfg.Models.Task.ReasoningEffort)
	}

	out, err = configSet(cfg, "models.task.verbosity", "low")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("expected ok response, got %q", out)
	}
	if cfg.Models.Task.Verbosity != "low" {
		t.Fatalf("expected models.task.verbosity low, got %q", cfg.Models.Task.Verbosity)
	}

	out, err = configSet(cfg, "models.task.reasoning_effort", "turbo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "invalid models.task.reasoning_effort") {
		t.Fatalf("expected validation error, got %q", out)
	}
}

func TestConfigSetRejectsInvalidPurposeModelFields(t *testing.T) {
	cfg := &Config{
		Home:      t.TempDir(),
		OpenAIKey: "sk-test",
		Models: ModelsConfig{
			Main: ModelConfig{
				Model: "gpt-5",
			},
			Critic: CriticConfig{
				Enabled: true,
				Model:   "gpt-4.1-nano",
			},
		},
	}

	out, err := configSet(cfg, "models.recall.reasoning_effort", "turbo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "invalid models.recall.reasoning_effort") {
		t.Fatalf("expected validation error, got %q", out)
	}

	out, err = configSet(cfg, "models.critic.model", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "missing required config key models.critic.model") {
		t.Fatalf("expected missing critic model error, got %q", out)
	}
	if cfg.Models.Critic.Model == "" {
		t.Fatal("expected rollback to keep prior models.critic.model")
	}
}

func TestConfigGetIncludesTaskModel(t *testing.T) {
	cfg := &Config{
		Models: ModelsConfig{
			Task: ModelConfig{Model: "gpt-4.1-mini", ReasoningEffort: "low"},
		},
	}

	out, err := configGet(cfg)
	if err != nil {
		t.Fatalf("config get: %v", err)
	}

	var data map[string]any
	err = json.Unmarshal([]byte(out), &data)
	if err != nil {
		t.Fatalf("unmarshal config get: %v", err)
	}

	models, ok := data["models"].(map[string]any)
	if !ok {
		t.Fatal("expected models key in config get output")
	}

	taskSection, ok := models["task"].(map[string]any)
	if !ok {
		t.Fatal("expected models.task key in config get output")
	}

	if taskSection["model"] != "gpt-4.1-mini" {
		t.Fatalf("expected task model gpt-4.1-mini, got %v", taskSection["model"])
	}
}

func TestConfigGetOmitsLegacyExaAPIKey(t *testing.T) {
	cfg := &Config{
		MaxHistory: 25,
		Timezone:   "UTC",
		Location:   "SF",
	}

	out, err := configGet(cfg)
	if err != nil {
		t.Fatalf("config get: %v", err)
	}

	var data map[string]any
	err = json.Unmarshal([]byte(out), &data)
	if err != nil {
		t.Fatalf("unmarshal config get: %v", err)
	}

	if _, ok := data["exa_api_key"]; ok {
		t.Fatal("expected config get to omit exa_api_key")
	}
}
