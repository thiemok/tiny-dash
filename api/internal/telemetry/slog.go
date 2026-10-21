package telemetry

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// traceContextHandler adds trace_id/span_id to stdout records so plain
// container logs can be correlated with traces even without the OTLP bridge.
type traceContextHandler struct {
	slog.Handler
}

func (h traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceContextHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceContextHandler) WithGroup(name string) slog.Handler {
	return traceContextHandler{h.Handler.WithGroup(name)}
}

type fanoutHandler []slog.Handler

func (f fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanoutHandler, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f fanoutHandler) WithGroup(name string) slog.Handler {
	out := make(fanoutHandler, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}
