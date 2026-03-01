package brain

import (
	"testing"

	"github.com/kciuffolo/nik/internal/config"
)

func TestNewInitializesInternalState(t *testing.T) {
	b := New(&config.Config{}, nil)
	if b == nil {
		t.Fatalf("expected non-nil brain")
	}
	if b.now == nil {
		t.Fatalf("expected now function to be initialized")
	}
	if b.toolExec == nil || b.privileged == nil {
		t.Fatalf("expected maps to be initialized")
	}
	if b.activeConversations == nil || b.activations == nil {
		t.Fatalf("expected sync sets to be initialized")
	}
	if len(b.toolDefs) != 0 || len(b.dataSources) != 0 {
		t.Fatalf("expected no tools or data sources on startup")
	}
}
