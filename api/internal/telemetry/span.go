package telemetry

import (
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RecordError marks the span as failed and attaches err as an exception event.
// It returns err so call sites can stay one line: `return telemetry.RecordError(span, err)`.
func RecordError(span trace.Span, err error) error {
	if err == nil {
		return nil
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return err
}
