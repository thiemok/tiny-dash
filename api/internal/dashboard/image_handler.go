package dashboard

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/thiemok/tiny-dash/api/internal/render"
	"github.com/thiemok/tiny-dash/api/internal/telemetry"
)

// ImageHandler serves the rendered e-ink image endpoints for picoDevice.
type ImageHandler struct {
	renderer   *render.Renderer
	baseURL    string
	settleTime time.Duration
}

// NewImageHandler creates an image handler.
// baseURL is the server's own address (e.g. "http://localhost:8080") used to
// load the dashboard in Chrome for screenshotting.
func NewImageHandler(renderer *render.Renderer, baseURL string) *ImageHandler {
	return &ImageHandler{
		renderer:   renderer,
		baseURL:    baseURL,
		settleTime: 500 * time.Millisecond,
	}
}

// RegisterRoutes registers the image API routes on the given mux.
func (h *ImageHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/dashboard/image", h.handleImage)
	mux.HandleFunc("GET /api/dashboard/preview", h.handlePreview)
}

func (h *ImageHandler) parseParams(r *http.Request) (width, height, colorDepth int, palette render.Palette, err error) {
	width, err = strconv.Atoi(r.URL.Query().Get("width"))
	if err != nil || width <= 0 {
		return 0, 0, 0, nil, fmt.Errorf("invalid width parameter")
	}

	height, err = strconv.Atoi(r.URL.Query().Get("height"))
	if err != nil || height <= 0 {
		return 0, 0, 0, nil, fmt.Errorf("invalid height parameter")
	}

	depthStr := r.URL.Query().Get("colorDepth")
	if depthStr != "" {
		colorDepth, err = strconv.Atoi(depthStr)
		if err != nil || colorDepth <= 0 {
			return 0, 0, 0, nil, fmt.Errorf("invalid colorDepth parameter")
		}
	}

	colorsParam := r.URL.Query().Get("colors")
	if colorsParam == "" {
		return 0, 0, 0, nil, fmt.Errorf("missing colors parameter")
	}

	colorStrs := strings.Split(colorsParam, ",")
	colorIndices := make([]byte, len(colorStrs))
	for i, cs := range colorStrs {
		c, cerr := strconv.Atoi(strings.TrimSpace(cs))
		if cerr != nil {
			return 0, 0, 0, nil, fmt.Errorf("invalid color value: %s", cs)
		}
		colorIndices[i] = byte(c)
	}

	palette = render.PaletteFromColors(colorIndices)
	return width, height, colorDepth, palette, nil
}

func (h *ImageHandler) dashboardURL(width, height int, colors string, extra ...string) string {
	url := fmt.Sprintf("%s/dashboard?width=%d&height=%d&colors=%s", h.baseURL, width, height, colors)
	for i := 0; i+1 < len(extra); i += 2 {
		if extra[i+1] != "" {
			url += "&" + extra[i] + "=" + extra[i+1]
		}
	}
	return url
}

// renderDithered screenshots the dashboard in Chrome and dithers it to the
// palette, recording the render duration for the given output format.
func (h *ImageHandler) renderDithered(ctx context.Context, r *http.Request, output string, width, height int, palette render.Palette) (*image.RGBA, error) {
	start := time.Now()
	attrs := []attribute.KeyValue{attrRenderOutput.String(output)}
	defer func() {
		renderDuration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attrs...))
	}()

	url := h.dashboardURL(width, height, r.URL.Query().Get("colors"), "mock", r.URL.Query().Get("mock"))
	screenshot, err := h.renderer.CaptureURL(ctx, url, width, height, h.settleTime)
	if err != nil {
		attrs = append(attrs, semconv.ErrorTypeKey.String("capture"))
		return nil, err
	}

	_, span := tracer.Start(ctx, "render.Dither", trace.WithAttributes(attribute.Int("tinydash.render.palette_size", len(palette))))
	defer span.End()
	return render.Dither(screenshot, palette), nil
}

func (h *ImageHandler) handleImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slog.InfoContext(ctx, "image request", "host", r.Host)

	width, height, colorDepth, palette, err := h.parseParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if colorDepth == 0 {
		http.Error(w, "missing colorDepth parameter", http.StatusBadRequest)
		return
	}

	dithered, err := h.renderDithered(ctx, r, "packed", width, height, palette)
	if err != nil {
		http.Error(w, "screenshot failed", http.StatusInternalServerError)
		slog.ErrorContext(ctx, "dashboard render failed", "error", err)
		return
	}

	_, span := tracer.Start(ctx, "render.PackImage", trace.WithAttributes(attribute.Int("tinydash.render.color_depth", colorDepth)))
	packed := render.PackImage(dithered, width, height, colorDepth, palette)
	span.End()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(packed)))
	w.Write(packed)

	slog.InfoContext(ctx, "sent packed image", "bytes", len(packed), "width", width, "height", height, "color_depth", colorDepth)
}

func (h *ImageHandler) handlePreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	width, height, _, palette, err := h.parseParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dithered, err := h.renderDithered(ctx, r, "png", width, height, palette)
	if err != nil {
		http.Error(w, "screenshot failed", http.StatusInternalServerError)
		slog.ErrorContext(ctx, "dashboard render failed", "error", err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, dithered); err != nil {
		telemetry.RecordError(trace.SpanFromContext(ctx), err)
		slog.ErrorContext(ctx, "png encode failed", "error", err)
	}
}
