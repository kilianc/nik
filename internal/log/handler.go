package log

import (
	"context"
	"log/slog"
)

const maxAttrLen = 20

type TruncHandler struct {
	Inner slog.Handler
}

func (h *TruncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Inner.Enabled(ctx, level)
}

func (h *TruncHandler) Handle(ctx context.Context, r slog.Record) error {
	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.truncAttr(a))
		return true
	})

	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	nr.AddAttrs(attrs...)
	return h.Inner.Handle(ctx, nr)
}

func (h *TruncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	t := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		t[i] = h.truncAttr(a)
	}
	return &TruncHandler{Inner: h.Inner.WithAttrs(t)}
}

func (h *TruncHandler) WithGroup(name string) slog.Handler {
	return &TruncHandler{Inner: h.Inner.WithGroup(name)}
}

func (h *TruncHandler) truncAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindString {
		s := a.Value.String()
		if len(s) > maxAttrLen {
			a.Value = slog.StringValue(s[:maxAttrLen] + "…")
		}
	}
	return a
}

// MultiHandler fans out each log record to all inner handlers.
type MultiHandler struct {
	Handlers []slog.Handler
}

func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.Handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.Handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		err := h.Handle(ctx, r)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.Handlers))
	for i, h := range m.Handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{Handlers: handlers}
}

func (m *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.Handlers))
	for i, h := range m.Handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{Handlers: handlers}
}
