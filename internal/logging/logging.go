// Package logging writes JSON logs through slog and carries per-request attributes on the context.
package logging

import (
	"context"
	"io"
	"log/slog"
)

type contextKey struct{}

// With returns a context whose log lines carry these attributes too.
func With(ctx context.Context, attrs ...slog.Attr) context.Context {
	carried, _ := ctx.Value(contextKey{}).([]slog.Attr)
	return context.WithValue(ctx, contextKey{}, append(append([]slog.Attr(nil), carried...), attrs...))
}

// New is a JSON logger that adds the context's attributes to every *Context call.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(contextHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if carried, ok := ctx.Value(contextKey{}).([]slog.Attr); ok {
		record.AddAttrs(carried...)
	}
	return h.Handler.Handle(ctx, record) //nolint:wrapcheck // the wrapped handler's error is this handler's
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
