package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestShortID(t *testing.T) {
	got := shortID("019d0e0c-79e9-7adb-be0f-5ceb8519bcfd")
	if got != "5ceb8519bcfd" {
		t.Errorf("shortID = %q, want %q", got, "5ceb8519bcfd")
	}
}

func TestTruncate(t *testing.T) {
	got := truncate("hello world", 5)
	if got != "hello..." {
		t.Errorf("truncate = %q, want %q", got, "hello...")
	}
}

func TestLoadCase(t *testing.T) {
	dir := t.TempDir()

	cj := caseJSON{
		Activations: []caseActivation{{
			ID:    "act-1",
			Model: "gpt-4o-mini",
			Tools: []string{"message_send", "message_noop"},
			Rounds: []caseRound{{
				Round:     0,
				InputFile: "round_0_input.txt",
			}},
		}},
		Diagnosis: caseDiagnosis{
			Category:     "SURFACED_NOT_ACTED",
			ActivationID: "act-1",
			Round:        0,
		},
	}

	data, _ := json.MarshalIndent(cj, "", "  ")
	os.WriteFile(filepath.Join(dir, "case.json"), data, 0o644)

	loaded, err := loadCase(dir)
	if err != nil {
		t.Fatalf("loadCase: %v", err)
	}

	if len(loaded.Activations) != 1 {
		t.Fatalf("activations = %d, want 1", len(loaded.Activations))
	}
	if loaded.Activations[0].Model != "gpt-4o-mini" {
		t.Errorf("model = %q, want gpt-4o-mini", loaded.Activations[0].Model)
	}
	if loaded.Diagnosis.Category != "SURFACED_NOT_ACTED" {
		t.Errorf("category = %q, want SURFACED_NOT_ACTED", loaded.Diagnosis.Category)
	}
}

func TestLoadToolSchemas(t *testing.T) {
	dir := t.TempDir()

	tools := []toolSchema{
		{Name: "message_send", Description: "Send a message", Parameters: map[string]any{"type": "object"}},
		{Name: "message_noop", Description: "No-op", Parameters: map[string]any{"type": "object"}},
	}

	data, _ := json.MarshalIndent(tools, "", "  ")
	os.WriteFile(filepath.Join(dir, "tools.json"), data, 0o644)

	loaded, err := loadToolSchemas(dir)
	if err != nil {
		t.Fatalf("loadToolSchemas: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("tools = %d, want 2", len(loaded))
	}
	if loaded[0].Name != "message_send" {
		t.Errorf("tool[0].name = %q, want message_send", loaded[0].Name)
	}
}
