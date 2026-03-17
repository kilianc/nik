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

func TestConfigGetOmitsLegacyExaAPIKey(t *testing.T) {
	cfg := &Config{
		MediaDirValue: "media",
		MaxHistory:    25,
		Timezone:      "UTC",
		Location:      "SF",
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
