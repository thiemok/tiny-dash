package dashboard

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/thiemok/tiny-dash/api/internal/render"
)

// DashboardHandler serves the browser-facing HTML dashboard and HTMX partials.
type DashboardHandler struct {
	tmpl      *template.Template
	startTime time.Time
}

// NewDashboardHandler creates a dashboard handler. fsys must contain templates/ at root.
func NewDashboardHandler(fsys fs.FS) (*DashboardHandler, error) {
	tmpl, err := loadTemplates(fsys)
	if err != nil {
		return nil, fmt.Errorf("loading templates: %w", err)
	}
	return &DashboardHandler{
		tmpl:      tmpl,
		startTime: time.Now(),
	}, nil
}

// templateData holds values passed to the dashboard template.
type templateData struct {
	Width       int
	Height      int
	Time        string
	Date        string
	RefreshTime string
	Uptime      string
	Swatches    []template.CSS
}

func (h *DashboardHandler) newTemplateData(width, height int, palette render.Palette) templateData {
	now := time.Now()
	uptime := now.Sub(h.startTime).Truncate(time.Minute)
	hours := int(uptime.Hours())
	minutes := int(uptime.Minutes()) % 60

	var uptimeStr string
	if hours > 0 {
		uptimeStr = fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		uptimeStr = fmt.Sprintf("%dm", minutes)
	}

	swatches := make([]template.CSS, len(palette))
	for i, e := range palette {
		swatches[i] = template.CSS(fmt.Sprintf("rgb(%d,%d,%d)", e.R, e.G, e.B))
	}

	return templateData{
		Width:       width,
		Height:      height,
		Time:        now.Format("15:04"),
		Date:        now.Format("Mon, 02 Jan '06"),
		RefreshTime: now.Format("15:04:05"),
		Uptime:      uptimeStr,
		Swatches:    swatches,
	}
}

// RegisterRoutes registers the dashboard HTML routes on the given mux.
func (h *DashboardHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard", h.handleDashboard)
	mux.HandleFunc("GET /dashboard/partials/clock", h.handleClock)
	mux.HandleFunc("GET /dashboard/partials/status", h.handleStatus)
}

func (h *DashboardHandler) parseDashboardParams(r *http.Request) (int, int, render.Palette) {
	width := 800
	height := 480
	colorIndices := []byte{0, 1, 2, 3, 4, 5, 6}

	if w, err := strconv.Atoi(r.URL.Query().Get("width")); err == nil && w > 0 {
		width = w
	}
	if ht, err := strconv.Atoi(r.URL.Query().Get("height")); err == nil && ht > 0 {
		height = ht
	}
	if colorsParam := r.URL.Query().Get("colors"); colorsParam != "" {
		colorStrs := strings.Split(colorsParam, ",")
		parsed := make([]byte, 0, len(colorStrs))
		for _, cs := range colorStrs {
			if c, err := strconv.Atoi(strings.TrimSpace(cs)); err == nil {
				parsed = append(parsed, byte(c))
			}
		}
		if len(parsed) > 0 {
			colorIndices = parsed
		}
	}

	return width, height, render.PaletteFromColors(colorIndices)
}

func (h *DashboardHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	width, height, palette := h.parseDashboardParams(r)
	data := h.newTemplateData(width, height, palette)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (h *DashboardHandler) handleClock(w http.ResponseWriter, r *http.Request) {
	data := h.newTemplateData(0, 0, nil)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "clock", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (h *DashboardHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	data := h.newTemplateData(0, 0, nil)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "status", data); err != nil {
		log.Printf("template error: %v", err)
	}
}
