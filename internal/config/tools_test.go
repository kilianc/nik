package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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
		AllowConversationIDs:      ConversationList{{Label: "owner", ID: "owner-conv"}},
		PrivilegedConversationIDs: ConversationList{{Label: "owner", ID: "owner-conv"}},
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
				Model:           "gpt-5",
				ReasoningEffort: "high",
			},
			Task: ModelConfig{
				ReasoningEffort: "xhigh",
			},
			Recall: ModelConfig{
				Model:           "gpt-4.1-nano",
				ReasoningEffort: "minimal",
			},
		},
		PrivilegedConversationIDs: ConversationList{{Label: "owner", ID: "conv-1"}},
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
}

func TestConfigSetSupportsTaskModelFields(t *testing.T) {
	cfg := &Config{
		Home:      t.TempDir(),
		OpenAIKey: "sk-test",
		Models: ModelsConfig{
			Main:   ModelConfig{Model: "gpt-5", ReasoningEffort: "high"},
			Task:   ModelConfig{ReasoningEffort: "xhigh"},
			Recall: ModelConfig{ReasoningEffort: "minimal"},
		},
		PrivilegedConversationIDs: ConversationList{{Label: "owner", ID: "conv-1"}},
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
		},
	}

	out, err := configSet(cfg, "models.recall.reasoning_effort", "turbo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "invalid models.recall.reasoning_effort") {
		t.Fatalf("expected validation error, got %q", out)
	}
}

func TestConfigGetTask(t *testing.T) {
	t.Run("includes task model", func(t *testing.T) {
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
	})

	t.Run("includes task settings", func(t *testing.T) {
		cfg := &Config{
			Task: TaskConfig{MaxRounds: 150, Timeout: 90 * time.Minute},
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

		taskSection, ok := data["task"].(map[string]any)
		if !ok {
			t.Fatal("expected task key in config get output")
		}

		if taskSection["max_rounds"] != float64(150) {
			t.Fatalf("expected task.max_rounds 150, got %v", taskSection["max_rounds"])
		}
		if taskSection["timeout"] != "1h30m0s" {
			t.Fatalf("expected task.timeout 1h30m0s, got %v", taskSection["timeout"])
		}
	})

	t.Run("defaults", func(t *testing.T) {
		cfg := &Config{}

		out, err := configGet(cfg)
		if err != nil {
			t.Fatalf("config get: %v", err)
		}

		var data map[string]any
		err = json.Unmarshal([]byte(out), &data)
		if err != nil {
			t.Fatalf("unmarshal config get: %v", err)
		}

		taskSection, ok := data["task"].(map[string]any)
		if !ok {
			t.Fatal("expected task key in config get output")
		}

		if taskSection["max_rounds"] != float64(200) {
			t.Fatalf("expected default task.max_rounds 200, got %v", taskSection["max_rounds"])
		}
		if taskSection["timeout"] != "1h0m0s" {
			t.Fatalf("expected default task.timeout 1h0m0s, got %v", taskSection["timeout"])
		}
	})
}

func TestConfigSetTaskFields(t *testing.T) {
	t.Run("max_rounds", func(t *testing.T) {
		cfg := &Config{
			Home:      t.TempDir(),
			OpenAIKey: "sk-test",
			Models: ModelsConfig{
				Main:   ModelConfig{Model: "gpt-5", ReasoningEffort: "high"},
				Task:   ModelConfig{ReasoningEffort: "xhigh"},
				Recall: ModelConfig{ReasoningEffort: "minimal"},
			},
			PrivilegedConversationIDs: ConversationList{{Label: "owner", ID: "conv-1"}},
		}

		out, err := configSet(cfg, "task.max_rounds", "250")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"ok":true`) {
			t.Fatalf("expected ok, got %q", out)
		}
		if cfg.Task.MaxRounds != 250 {
			t.Fatalf("expected 250, got %d", cfg.Task.MaxRounds)
		}

		out, _ = configSet(cfg, "task.max_rounds", "0")
		if !strings.Contains(out, "error") {
			t.Fatalf("expected validation error for 0, got %q", out)
		}

		out, _ = configSet(cfg, "task.max_rounds", "abc")
		if !strings.Contains(out, "invalid task.max_rounds") {
			t.Fatalf("expected parse error, got %q", out)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		cfg := &Config{
			Home:      t.TempDir(),
			OpenAIKey: "sk-test",
			Models: ModelsConfig{
				Main:   ModelConfig{Model: "gpt-5", ReasoningEffort: "high"},
				Task:   ModelConfig{ReasoningEffort: "xhigh"},
				Recall: ModelConfig{ReasoningEffort: "minimal"},
			},
			PrivilegedConversationIDs: ConversationList{{Label: "owner", ID: "conv-1"}},
		}

		out, err := configSet(cfg, "task.timeout", "90m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"ok":true`) {
			t.Fatalf("expected ok, got %q", out)
		}
		if cfg.Task.Timeout != 90*time.Minute {
			t.Fatalf("expected 90m, got %v", cfg.Task.Timeout)
		}

		out, _ = configSet(cfg, "task.timeout", "30s")
		if !strings.Contains(out, "error") {
			t.Fatalf("expected validation error for 30s, got %q", out)
		}

		out, _ = configSet(cfg, "task.timeout", "not-a-duration")
		if !strings.Contains(out, "invalid task.timeout") {
			t.Fatalf("expected parse error, got %q", out)
		}
	})
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
