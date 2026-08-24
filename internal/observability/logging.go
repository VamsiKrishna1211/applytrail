// Package observability wires structured logging, request correlation, and
// OpenTelemetry.
//
// Telemetry export is opt-in: with OTEL_EXPORTER_OTLP_ENDPOINT unset, Setup
// installs no exporters and the process emits nothing over the network. That
// keeps a self-hosted or free-tier deployment at zero cost by default while
// leaving a single env var between it and full traces and metrics.
package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// NewLogger builds the process logger: JSON to stdout, which Cloud Logging and
// most log pipelines parse without additional configuration.
func NewLogger(level string, addSource bool) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     parseLevel(level),
		AddSource: addSource,
	})
	return slog.New(&contextHandler{Handler: handler})
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// contextHandler decorates every record with the correlation fields carried on
// the context, so handlers never have to thread a request ID through manually.
type contextHandler struct{ slog.Handler }

func (h *contextHandler) Handle(ctx context.Context, rec slog.Record) error {
	if id := RequestIDFromContext(ctx); id != "" {
		rec.AddAttrs(slog.String("request_id", id))
	}
	if p := principalAttrs(ctx); p != nil {
		rec.AddAttrs(p...)
	}
	return h.Handler.Handle(ctx, rec)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

// logFieldsKey is the context key under which callers may stash extra slog
// attributes to be attached to every subsequent log line. The auth middleware
// uses it to record which API key made a request.
type logFieldsKey struct{}

// ContextWithLogFields returns a context carrying attrs on every log record.
func ContextWithLogFields(ctx context.Context, attrs ...slog.Attr) context.Context {
	existing, _ := ctx.Value(logFieldsKey{}).([]slog.Attr)
	merged := make([]slog.Attr, 0, len(existing)+len(attrs))
	merged = append(merged, existing...)
	merged = append(merged, attrs...)
	return context.WithValue(ctx, logFieldsKey{}, merged)
}

func principalAttrs(ctx context.Context) []slog.Attr {
	attrs, _ := ctx.Value(logFieldsKey{}).([]slog.Attr)
	return attrs
}
