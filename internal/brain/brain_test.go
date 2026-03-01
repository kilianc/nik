package brain

import (
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
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
	if b.active == nil || b.runs == nil {
		t.Fatalf("expected sync sets to be initialized")
	}
	if len(b.toolDefs) != 0 || len(b.dataSources) != 0 {
		t.Fatalf("expected no tools or data sources on startup")
	}
}

func TestEnsureToolCallsRejectsEmpty(t *testing.T) {
	err := ensureToolCalls(nil)
	if err == nil {
		t.Fatalf("expected error for empty tool calls")
	}
}

func TestEnsureToolCallsAcceptsNonEmpty(t *testing.T) {
	err := ensureToolCalls([]llm.ToolCallRecord{
		{Name: "message_noop"},
	})
	if err != nil {
		t.Fatalf("expected non-empty tool calls to pass, got %v", err)
	}
}
