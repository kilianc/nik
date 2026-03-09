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
	if b.claimed == nil {
		t.Fatalf("expected sync set to be initialized")
	}
	if len(b.toolDefs) != 0 {
		t.Fatalf("expected no tools on startup")
	}
	if b.sensor != nil {
		t.Fatalf("expected sensor to be nil on startup")
	}
}

func TestHasTerminalCall(t *testing.T) {
	tests := []struct {
		name    string
		history []llm.ToolCallRecord
		want    bool
	}{
		{
			name:    "empty history",
			history: nil,
			want:    false,
		},
		{
			name:    "only non-terminal calls",
			history: []llm.ToolCallRecord{{Name: "task_list"}, {Name: "db_query"}},
			want:    false,
		},
		{
			name:    "message_reply present",
			history: []llm.ToolCallRecord{{Name: "task_list"}, {Name: "message_reply"}},
			want:    true,
		},
		{
			name:    "message_noop present",
			history: []llm.ToolCallRecord{{Name: "message_noop"}},
			want:    true,
		},
		{
			name:    "message_react present",
			history: []llm.ToolCallRecord{{Name: "load_skill"}, {Name: "message_react"}},
			want:    true,
		},
		{
			name:    "non-terminal messaging tools are not terminal",
			history: []llm.ToolCallRecord{{Name: "message_set_presence"}, {Name: "message_update_media_description"}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTerminalCall(tt.history)
			if got != tt.want {
				t.Errorf("hasTerminalCall() = %v, want %v", got, tt.want)
			}
		})
	}
}
