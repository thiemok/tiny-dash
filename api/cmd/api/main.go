package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	api "github.com/thiemok/tiny-dash/api"
	"github.com/thiemok/tiny-dash/api/internal/config"
	"github.com/thiemok/tiny-dash/api/internal/dashboard"
	"github.com/thiemok/tiny-dash/api/internal/homeassistant"
	"github.com/thiemok/tiny-dash/api/internal/render"
	"github.com/thiemok/tiny-dash/api/internal/telemetry"
)

const healthPath = "/api/hello"

func main() {
	ctx := context.Background()

	shutdownTelemetry, err := telemetry.Setup(ctx)
	if err != nil {
		fatal(ctx, "failed to set up telemetry", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(ctx); err != nil {
			slog.ErrorContext(ctx, "telemetry shutdown failed", "error", err)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		fatal(ctx, "failed to load config", err)
	}

	haClient := homeassistant.NewClient(cfg.HA.BaseURL, cfg.HA.Token)

	chromePath := os.Getenv("CHROME_PATH")
	if chromePath != "" {
		slog.InfoContext(ctx, "using custom Chrome executable", "path", chromePath)
	}
	renderer, err := render.NewRenderer(chromePath)
	if err != nil {
		fatal(ctx, "failed to start renderer", err)
	}
	defer renderer.Close()

	dashHandler, err := dashboard.NewDashboardHandler(api.TemplateFS, haClient, cfg)
	if err != nil {
		fatal(ctx, "failed to create dashboard handler", err)
	}

	port := ":8080"
	baseURL := "http://localhost" + port

	imageHandler := dashboard.NewImageHandler(renderer, baseURL)

	slog.InfoContext(ctx, "Starting API server on "+port)
	slog.InfoContext(ctx, "Endpoints:")
	slog.InfoContext(ctx, "  GET /dashboard                      — HTML dashboard")
	slog.InfoContext(ctx, "  GET /api/dashboard/image             — packed e-ink image")
	slog.InfoContext(ctx, "  GET /api/dashboard/preview           — PNG preview of the panel image")
	slog.InfoContext(ctx, "  GET "+healthPath+"                       — health check")

	srv := &http.Server{Addr: port, Handler: newHandler(dashHandler, imageHandler)}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal(ctx, "server failed", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.InfoContext(ctx, "Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(ctx, "graceful shutdown failed", "error", err)
	}
}

// newHandler builds the routed, instrumented HTTP handler. Every route is
// registered on the inner mux, so otelhttp traces and measures all of them;
// span names and http.route come from the matched ServeMux pattern.
func newHandler(dashHandler *dashboard.DashboardHandler, imageHandler *dashboard.ImageHandler) http.Handler {
	mux := http.NewServeMux()
	dashHandler.RegisterRoutes(mux)
	imageHandler.RegisterRoutes(mux)

	mux.HandleFunc("GET "+healthPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Hello, World!"))
	})

	mux.Handle("GET /static/", http.FileServerFS(api.StaticFS))

	return otelhttp.NewHandler(withRouteAttribute(mux), "http.server",
		// Liveness/readiness probes hit the health check every few seconds; tracing them is noise.
		otelhttp.WithFilter(func(r *http.Request) bool { return r.URL.Path != healthPath }),
	)
}

// withRouteAttribute copies the matched ServeMux pattern onto the server span as
// http.route; otelhttp only applies it to the span name and metrics.
func withRouteAttribute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if i := strings.Index(r.Pattern, "/"); i >= 0 {
			trace.SpanFromContext(r.Context()).SetAttributes(semconv.HTTPRoute(r.Pattern[i:]))
		}
	})
}

func fatal(ctx context.Context, msg string, err error) {
	slog.ErrorContext(ctx, msg, "error", err)
	os.Exit(1)
}
