package daemonctl

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	"github.com/kciuffolo/nik/internal/llm"
)

func TestRestartHandler(t *testing.T) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	defer signal.Stop(sig)

	handler := RestartHandler()

	result, err := handler(context.Background(), llm.ToolCall{Name: "restart", Arguments: "{}"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !strings.Contains(result, "restart scheduled") {
		t.Fatalf("expected restart message, got %s", result)
	}

	got := <-sig
	if got != syscall.SIGTERM {
		t.Fatalf("expected SIGTERM, got %v", got)
	}
}
