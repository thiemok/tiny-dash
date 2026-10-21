// Package telemetry wires up the OpenTelemetry SDK (traces, metrics, logs) for
// the API. Conventions for using it live in api/docs/observability.md.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// ServiceName is the default service.name; OTEL_SERVICE_NAME overrides it.
const ServiceName = "tiny-dash-api"

// ScopeName is the instrumentation scope for API-wide telemetry such as logs.
const ScopeName = "github.com/thiemok/tiny-dash/api"

// Version is the service.version, stamped at build time via
// -ldflags "-X github.com/thiemok/tiny-dash/api/internal/telemetry.Version=...".
var Version = "dev"

// Setup installs the global tracer, meter and logger providers plus the W3C
// propagators, and makes slog the default logger (stdout, plus OTLP when
// export is enabled). The returned function flushes and shuts everything down.
//
// Export is configured purely through the standard OTEL_* environment
// variables. When neither an OTLP endpoint nor an explicit exporter is set,
// the SDK is not installed at all, so local runs stay quiet and free.
func Setup(ctx context.Context) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	stdout := slog.NewTextHandler(os.Stdout, nil)

	if !exportEnabled() {
		slog.SetDefault(slog.New(traceContextHandler{stdout}))
		slog.InfoContext(ctx, "telemetry export disabled; set OTEL_EXPORTER_OTLP_ENDPOINT to enable")
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(ServiceName),
			semconv.ServiceVersion(Version),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
	)
	if err != nil {
		return nil, fmt.Errorf("building resource: %w", err)
	}

	var shutdowns []func(context.Context) error
	shutdown := func(ctx context.Context) error {
		var errs []error
		for i := len(shutdowns) - 1; i >= 0; i-- {
			errs = append(errs, shutdowns[i](ctx))
		}
		return errors.Join(errs...)
	}
	fail := func(err error) (func(context.Context) error, error) {
		return nil, errors.Join(err, shutdown(ctx))
	}

	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return fail(fmt.Errorf("creating span exporter: %w", err))
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(spanExporter),
	)
	shutdowns = append(shutdowns, tp.Shutdown)
	otel.SetTracerProvider(tp)

	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return fail(fmt.Errorf("creating metric reader: %w", err))
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(metricReader),
	)
	shutdowns = append(shutdowns, mp.Shutdown)
	otel.SetMeterProvider(mp)

	logExporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return fail(fmt.Errorf("creating log exporter: %w", err))
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)
	shutdowns = append(shutdowns, lp.Shutdown)
	global.SetLoggerProvider(lp)

	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(15 * time.Second)); err != nil {
		return fail(fmt.Errorf("starting runtime metrics: %w", err))
	}

	slog.SetDefault(slog.New(fanoutHandler{
		traceContextHandler{stdout},
		otelslog.NewHandler(ScopeName, otelslog.WithLoggerProvider(lp)),
	}))

	return shutdown, nil
}

func exportEnabled() bool {
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		return false
	}
	for _, key := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_TRACES_EXPORTER",
		"OTEL_METRICS_EXPORTER",
		"OTEL_LOGS_EXPORTER",
	} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}
