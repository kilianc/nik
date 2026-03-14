package config

import (
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
