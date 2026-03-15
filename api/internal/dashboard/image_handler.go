package dashboard

import (
	"fmt"
	"image/png"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/thiemok/tiny-dash/api/internal/render"
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

func (h *ImageHandler) dashboardURL(width, height int, colors string) string {
	return fmt.Sprintf("%s/dashboard?width=%d&height=%d&colors=%s", h.baseURL, width, height, colors)
}

func (h *ImageHandler) handleImage(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %s", r.URL.String())

	width, height, colorDepth, palette, err := h.parseParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if colorDepth == 0 {
		http.Error(w, "missing colorDepth parameter", http.StatusBadRequest)
		return
	}

	colorsParam := r.URL.Query().Get("colors")
	screenshot, err := h.renderer.CaptureURL(h.dashboardURL(width, height, colorsParam), width, height, h.settleTime)
	if err != nil {
		http.Error(w, "screenshot failed", http.StatusInternalServerError)
		log.Printf("screenshot error: %v", err)
		return
	}

	dithered := render.Dither(screenshot, palette)
	packed := render.PackImage(dithered, width, height, colorDepth, palette)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(packed)))
	w.Write(packed)

	log.Printf("Sent %d bytes (%dx%d, depth=%d)", len(packed), width, height, colorDepth)
}

func (h *ImageHandler) handlePreview(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received preview request: %s", r.URL.String())

	width, height, _, palette, err := h.parseParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	colorsParam := r.URL.Query().Get("colors")
	screenshot, err := h.renderer.CaptureURL(h.dashboardURL(width, height, colorsParam), width, height, h.settleTime)
	if err != nil {
		http.Error(w, "screenshot failed", http.StatusInternalServerError)
		log.Printf("screenshot error: %v", err)
		return
	}

	dithered := render.Dither(screenshot, palette)

	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, dithered); err != nil {
		log.Printf("png encode error: %v", err)
	}
}
