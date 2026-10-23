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

// designColors are the panel colors the dashboard template paints with
// (black, white, yellow, red, blue, green). When the requested palette covers
// all of them, the screenshot is palette colors plus antialiasing edges, and
// snapping each pixel to its nearest color is crisper than dithering.
var designColors = []byte{0, 1, 2, 3, 5, 6}

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

// renderImage screenshots the dashboard in Chrome and reduces it to the
// palette, recording the render duration for the given output format. It only
// dithers when the palette lacks colors the template uses; otherwise it maps
// each pixel to its nearest palette color.
func (h *ImageHandler) renderImage(ctx context.Context, r *http.Request, output string, width, height int, palette render.Palette) (*image.RGBA, map[string]string, error) {
	start := time.Now()
	dither := !palette.Covers(designColors)
	attrs := []attribute.KeyValue{attrRenderOutput.String(output), attrRenderDither.Bool(dither)}
	defer func() {
		renderDuration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attrs...))
	}()

	url := h.dashboardURL(width, height, r.URL.Query().Get("colors"), "mock", r.URL.Query().Get("mock"))
	screenshot, meta, err := h.renderer.CaptureURL(ctx, url, width, height, h.settleTime)
	if err != nil {
		attrs = append(attrs, semconv.ErrorTypeKey.String("capture"))
		return nil, nil, err
	}

	paletteSize := trace.WithAttributes(attribute.Int("tinydash.render.palette_size", len(palette)))
	if !dither {
		_, span := tracer.Start(ctx, "render.Quantize", paletteSize)
		defer span.End()
		return render.Quantize(screenshot, palette), meta, nil
	}
	_, span := tracer.Start(ctx, "render.Dither", paletteSize)
	defer span.End()
	return render.Dither(screenshot, palette), meta, nil
}

// setRefreshHint sets X-Refresh-After to the whole seconds, rounded up, until
// the page's next refresh time. Without a usable time it sets nothing and the
// device falls back to its own interval.
func setRefreshHint(ctx context.Context, w http.ResponseWriter, meta map[string]string, now time.Time) {
	at, err := time.Parse(time.RFC3339, meta[nextRefreshMeta])
	if err != nil {
		slog.WarnContext(ctx, "dashboard page has no usable next refresh time", "error", err)
		return
	}
	secs := max(int64((at.Sub(now)+time.Second-1)/time.Second), 0)
	w.Header().Set(refreshHintHeader, strconv.FormatInt(secs, 10))
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

	img, meta, err := h.renderImage(ctx, r, "packed", width, height, palette)
	if err != nil {
		http.Error(w, "screenshot failed", http.StatusInternalServerError)
		slog.ErrorContext(ctx, "dashboard render failed", "error", err)
		return
	}

	_, span := tracer.Start(ctx, "render.PackImage", trace.WithAttributes(attribute.Int("tinydash.render.color_depth", colorDepth)))
	packed := render.PackImage(img, width, height, colorDepth, palette)
	span.End()

	setRefreshHint(ctx, w, meta, time.Now())
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

	img, meta, err := h.renderImage(ctx, r, "png", width, height, palette)
	if err != nil {
		http.Error(w, "screenshot failed", http.StatusInternalServerError)
		slog.ErrorContext(ctx, "dashboard render failed", "error", err)
		return
	}

	setRefreshHint(ctx, w, meta, time.Now())
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		telemetry.RecordError(trace.SpanFromContext(ctx), err)
		slog.ErrorContext(ctx, "png encode failed", "error", err)
	}
}
