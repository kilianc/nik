package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestTruncHandler(t *testing.T) {
	t.Run("truncates long strings", func(t *testing.T) {
		var buf bytes.Buffer
		inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{})
		h := &TruncHandler{Inner: inner}
		logger := slog.New(h)

		long := strings.Repeat("x", 50)
		logger.Info("test", "key", long)

		out := buf.String()
		if strings.Contains(out, long) {
			t.Errorf("expected truncation, full string found in output")
		}
		truncated := strings.Repeat("x", maxAttrLen) + "…"
		if !strings.Contains(out, truncated) {
			t.Errorf("expected truncated string %q in output: %s", truncated, out)
		}
	})

	t.Run("preserves short strings", func(t *testing.T) {
		var buf bytes.Buffer
		inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{})
		h := &TruncHandler{Inner: inner}
		logger := slog.New(h)

		logger.Info("test", "key", "short")
		if !strings.Contains(buf.String(), "short") {
			t.Errorf("short string should be preserved: %s", buf.String())
		}
	})
}

func TestMultiHandlerFansOut(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	h1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{})
	h2 := slog.NewTextHandler(&buf2, &slog.HandlerOptions{})
	multi := &MultiHandler{Handlers: []slog.Handler{h1, h2}}
	logger := slog.New(multi)

	logger.Info("hello")

	if !strings.Contains(buf1.String(), "hello") {
		t.Errorf("handler 1 missing message: %s", buf1.String())
	}
	if !strings.Contains(buf2.String(), "hello") {
		t.Errorf("handler 2 missing message: %s", buf2.String())
	}
}

func TestMultiHandlerEnabledAny(t *testing.T) {
	warn := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	debug := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	multi := &MultiHandler{Handlers: []slog.Handler{warn, debug}}

	if !multi.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("should be enabled when at least one handler accepts the level")
	}
}
