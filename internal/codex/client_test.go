package codex

import (
	"testing"
)

func TestBuildOpenAIClientWithKey(t *testing.T) {
	client, err := BuildOpenAIClient("test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
