package log

import (
	"context"
	"log/slog"
)

const maxAttrLen = 20

// TruncHandler wraps an slog.Handler and truncates all string attribute
// values to maxAttrLen characters.
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
