# Observability (OpenTelemetry)

The API emits **traces, metrics and logs** through the OpenTelemetry Go SDK and exports them over
OTLP. This document holds the conventions every backend change must follow. The static parts are
enforced by `api/conventions_test.go`; the rest is on the author (human or agent).

## How it is wired

| Concern | Where | Notes |
|---|---|---|
| SDK setup | `internal/telemetry/telemetry.go` (`telemetry.Setup`) | Called first thing in `main`; shutdown flushes on exit. |
| Inbound HTTP | `cmd/api/main.go` (`newHandler`) | One `otelhttp.NewHandler` around the whole `ServeMux`. Span name = `METHOD /pattern`, `http.route` = pattern. `/api/hello` (probes) is filtered out. |
| Outbound HTTP | `internal/homeassistant/client.go` | `http.Client{Transport: otelhttp.NewTransport(...)}` + `NewRequestWithContext`: client spans, metrics and `traceparent` injection. |
| Chrome render | `internal/render/screenshot.go` | `CaptureURL` sends the caller's `traceparent` as an extra header on every request Chrome makes, so the self-request to `/dashboard` is a child of the image request. |
| Logs | `log/slog` default logger | Stdout text handler (adds `trace_id`/`span_id`) and, when export is on, the `otelslog` bridge (OTLP logs with trace context). |
| Runtime metrics | `go.opentelemetry.io/contrib/instrumentation/runtime` | `go.memory.*`, `go.goroutine.count`, … |

A request to `/api/dashboard/image` produces a single trace:

```
GET /api/dashboard/image
├─ render.CaptureURL
│  ├─ GET /dashboard                 (Chrome → API, joined via traceparent)
│  │  ├─ dashboard.fetch weather → homeassistant.GetWeather → homeassistant.GetState → HTTP GET
│  │  ├─ dashboard.fetch calendar → homeassistant.GetCalendarEventsForToday → …
│  │  └─ dashboard.fetch departures → homeassistant.GetUpcomingDepartures → …
│  └─ GET /static/
├─ render.Dither
└─ render.PackImage
```

## Configuration

Export is configured only through the standard `OTEL_*` environment variables. If no OTLP endpoint
and no `OTEL_{TRACES,METRICS,LOGS}_EXPORTER` is set, the SDK is not installed at all (logs still go
to stdout). `OTEL_SDK_DISABLED=true` forces it off.

| Variable | Typical value |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://otel-collector:4318` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` (default) or `grpc` |
| `OTEL_EXPORTER_OTLP_HEADERS` | `Authorization=Bearer …` for hosted backends (keep in a Secret) |
| `OTEL_RESOURCE_ATTRIBUTES` | `deployment.environment.name=production` |
| `OTEL_SERVICE_NAME` | overrides the default `tiny-dash-api` |
| `OTEL_TRACES_EXPORTER` / `OTEL_METRICS_EXPORTER` / `OTEL_LOGS_EXPORTER` | `otlp` (default), `console`, `none` |
| `OTEL_TRACES_SAMPLER` | default `parentbased_always_on`; traffic is low, keep everything |

Resource attributes: `service.name` (`tiny-dash-api`), `service.version` (stamped via
`-ldflags -X …/internal/telemetry.Version=…`; the Docker build takes `--build-arg VERSION`, which
the release workflow sets), plus host/process/SDK detectors. The Helm chart adds `k8s.pod.name`,
`k8s.namespace.name` and `deployment.environment.name` (`otel.environment`); see the `otel:`
block in `chart/values.yaml`.

## Conventions

### Context

- Every function that does I/O, or that is worth a span, takes `ctx context.Context` as its first
  parameter. Handlers use `r.Context()`. Never start from `context.Background()` inside a request.
- Outbound HTTP: `http.NewRequestWithContext` and a client whose `Transport` is
  `otelhttp.NewTransport(...)`. Never `http.Get`, `http.DefaultClient` or a bare `&http.Client{}`.

### Tracers and meters

- One tracer (and meter, if needed) per package, named after the package import path:

  ```go
  var tracer = otel.Tracer("github.com/thiemok/tiny-dash/api/internal/<pkg>")
  ```

  Package-level is fine: the global provider delegates once `telemetry.Setup` has run.

### Span names

- Inbound HTTP: automatic (`GET /api/dashboard/image`). Do not add a second span in the handler.
- Internal operations: `<package>.<Function>`, using the Go identifier (`homeassistant.GetWeather`,
  `render.CaptureURL`). For one function looped over a small, fixed set, use
  `<package>.<operation> <variant>` with the variant also recorded as an attribute
  (`dashboard.fetch weather` + `tinydash.datasource=weather`).
- Names must be low-cardinality: no IDs, URLs, user input or timestamps in span names.

### Attributes

- Prefer semantic-convention keys (`go.opentelemetry.io/otel/semconv/v1.43.0`) when one exists,
  e.g. `http.route` or `error.type`.
- Otherwise namespace custom keys: `tinydash.*` for domain concepts and `homeassistant.*` for the
  Home Assistant integration. Declare them once per package as `attribute.Key` vars
  (see `internal/dashboard/telemetry.go`).
- Bounded cardinality only: config-defined entity IDs, palette size and dimensions are fine;
  free-form text is not.

### Errors

- When a span's operation fails, call `telemetry.RecordError(span, err)`. It records the
  exception event, sets status `Error`, and returns `err`, so
  `return nil, telemetry.RecordError(span, fmt.Errorf(...))` stays a one-liner.
- When an error is handled and degraded gracefully (e.g. one calendar failing), log it at `Warn`
  with `slog.WarnContext`; mark the span as failed only if *its* operation failed.

### Logs

- Use `log/slog` and always the `*Context` variants (`slog.InfoContext(ctx, …)`), so records carry
  `trace_id`/`span_id`. Never import the stdlib `log` package.
- Message = constant, lower-case event description; variable data goes in key/value attributes
  (`"entity_id", id, "error", err`). Don't format values into the message.

### Metrics

- HTTP server/client metrics come from otelhttp; don't duplicate them.
- Domain metrics are named `tinydash.<area>.<measure>` with a UCUM unit (`s`, `By`, `{failure}`),
  created once at package level. Current set:

  | Metric | Type | Attributes | Meaning |
  |---|---|---|---|
  | `tinydash.render.duration` | histogram, `s` | `tinydash.render.output` (`packed`/`png`), `error.type` | Chrome capture + dither, per image request |
  | `tinydash.datasource.failures` | counter, `{failure}` | `tinydash.datasource` | A dashboard section rendered empty because its fetch failed |

### Never put in telemetry

- **Secrets**: the HA token, `Authorization` headers, `OTEL_EXPORTER_OTLP_HEADERS`. otelhttp does
  not record headers; keep it that way.
- **Personal data**: calendar event summaries/locations and other entity *content*. Entity IDs are
  fine, their state payloads are not.
- **High-cardinality values** as attributes or metric labels: full URLs with query strings,
  timestamps, request IDs.

## Copy-paste: a new endpoint

Register it on the mux in `RegisterRoutes`; otelhttp traces and measures it automatically. Only add
spans for meaningful sub-operations:

```go
func (h *Handler) handleThing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	thing, err := h.loadThing(ctx, r.PathValue("id"))
	if err != nil {
		http.Error(w, "load failed", http.StatusBadGateway)
		slog.ErrorContext(ctx, "thing load failed", "error", err)
		return
	}
	// ...
}

func (h *Handler) loadThing(ctx context.Context, id string) (*Thing, error) {
	ctx, span := tracer.Start(ctx, "mypkg.loadThing")
	defer span.End()

	body, err := h.client.Get(ctx, "/things/"+id) // instrumented client, ctx passed through
	if err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("loading thing: %w", err))
	}
	// ...
}
```

## Copy-paste: a new background job

There's no inbound request, so the job starts its own root span per run:

```go
func (j *Job) runOnce(ctx context.Context) {
	ctx, span := tracer.Start(ctx, "mypkg.Job.runOnce", trace.WithNewRoot())
	defer span.End()

	if err := j.do(ctx); err != nil {
		telemetry.RecordError(span, err)
		slog.ErrorContext(ctx, "job run failed", "error", err)
	}
}
```

## Definition of done for backend changes

- [ ] New I/O paths take `ctx` and pass it all the way down.
- [ ] New outbound HTTP goes through an `otelhttp` transport; new routes are registered on the
      instrumented mux (never a second `http.Server` or handler outside `newHandler`).
- [ ] Significant internal operations have a span named per the conventions above, and failures
      call `telemetry.RecordError`.
- [ ] Logs use `slog.*Context`; no secrets, PII or high-cardinality values in telemetry.
- [ ] `pnpm nx run api:test` passes (includes `TestInstrumentationConventions`).
- [ ] For non-trivial changes, the trace was looked at locally (below).

## Verifying locally

Use [otel-tui](https://github.com/ymtdzzz/otel-tui), a terminal OTLP receiver that shows traces
(as a waterfall), metrics and logs. It listens on `4317` (gRPC) and `4318` (HTTP).

```sh
brew install ymtdzzz/tap/otel-tui     # or: go install github.com/ymtdzzz/otel-tui@latest
otel-tui                              # in a separate terminal
```

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=local \
  pnpm nx run api:serve
curl -fs 'localhost:8080/api/dashboard/image?width=800&height=480&colors=0,1,2,3&colorDepth=4&mock=1' >/dev/null
```

Check that the image request appears in otel-tui as a single trace with the tree shown above, that
logs carry its trace ID, and that the `tinydash.*` metrics appear.

Without otel-tui, `OTEL_TRACES_EXPORTER=console OTEL_METRICS_EXPORTER=none OTEL_LOGS_EXPORTER=none`
prints spans to stdout.
