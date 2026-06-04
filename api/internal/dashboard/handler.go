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

	"github.com/thiemok/tiny-dash/api/internal/config"
	"github.com/thiemok/tiny-dash/api/internal/homeassistant"
	"github.com/thiemok/tiny-dash/api/internal/render"
)

// DashboardHandler serves the browser-facing HTML dashboard and HTMX partials.
type DashboardHandler struct {
	tmpl     *template.Template
	haClient *homeassistant.Client
	config   *config.Config
}

// NewDashboardHandler creates a dashboard handler. fsys must contain templates/ at root.
func NewDashboardHandler(fsys fs.FS, haClient *homeassistant.Client, cfg *config.Config) (*DashboardHandler, error) {
	tmpl, err := loadTemplates(fsys)
	if err != nil {
		return nil, fmt.Errorf("loading templates: %w", err)
	}
	return &DashboardHandler{
		tmpl:     tmpl,
		haClient: haClient,
		config:   cfg,
	}, nil
}

// hourlyForecastView holds a single hourly forecast entry for the template.
type hourlyForecastView struct {
	TimeStr     string
	Temperature float64
	Icon        string
}

// weatherView holds weather data for template rendering.
type weatherView struct {
	Temperature    float64
	Condition      string
	Icon           string
	TempHigh       float64
	TempLow        float64
	SunriseStr     string
	SunsetStr      string
	HourlyForecast []hourlyForecastView
}

// eventView holds a single calendar event for the template.
type eventView struct {
	TimeStr  string
	Summary  string
	Location string
	AllDay   bool
	Ongoing  bool
	Source   string         // short calendar label
	Color    template.CSS   // accent color for the calendar source
}

// calendarView holds events for today, grouped for display.
type calendarView struct {
	AllDay []eventView
	Timed  []eventView
}

// conditionClass maps HA weather condition strings to CSS class names for icons.
var conditionClass = map[string]string{
	"sunny":           "wi-sunny",
	"clear-night":     "wi-night",
	"partlycloudy":    "wi-partlycloudy",
	"cloudy":          "wi-cloudy",
	"rainy":           "wi-rainy",
	"pouring":         "wi-pouring",
	"snowy":           "wi-snowy",
	"snowy-rainy":     "wi-snowy",
	"fog":             "wi-fog",
	"hail":            "wi-rainy",
	"lightning":       "wi-lightning",
	"lightning-rainy": "wi-lightning",
	"windy":           "wi-windy",
	"windy-variant":   "wi-windy",
	"exceptional":     "wi-cloudy",
}

// templateData holds values passed to the dashboard template.
type templateData struct {
	Width    int
	Height   int
	Time     string
	Hour     string
	Dots     []bool // 12 entries, true = filled (elapsed 5-min block)
	Date     string
	Swatches []template.CSS
	Weather  *weatherView
	Calendar *calendarView
}

func minuteDots(minute int) []bool {
	filled := minute / 5
	dots := make([]bool, 12)
	for i := range filled {
		dots[i] = true
	}
	return dots
}

func (h *DashboardHandler) newTemplateData(width, height int, palette render.Palette) templateData {
	now := time.Now()

	swatches := make([]template.CSS, len(palette))
	for i, e := range palette {
		swatches[i] = template.CSS(fmt.Sprintf("rgb(%d,%d,%d)", e.R, e.G, e.B))
	}

	data := templateData{
		Width:    width,
		Height:   height,
		Time:     now.Format("15:04"),
		Hour:     now.Format("15"),
		Dots:     minuteDots(now.Minute()),
		Date:     now.Format("Mon, 02 Jan '06"),
		Swatches: swatches,
	}

	data.Weather = h.fetchWeather()
	data.Calendar = h.fetchCalendar(now)

	return data
}

// calendarPalette assigns an accent color per source calendar (cycled).
// Keeps the count low to stay within the limited color budget.
var calendarPalette = []template.CSS{
	"#0044cc", // blue
	"#c00000", // red
	"#008055", // green
	"#e08000", // orange
}

// shortCalendarLabel derives a short, human-readable label from an HA
// calendar entity_id (e.g. "calendar.feiertage_in_deutschland" -> "Feiertage").
func shortCalendarLabel(entityID string) string {
	name := entityID
	if idx := strings.Index(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.Index(name, "_"); idx >= 0 {
		name = name[:idx]
	}
	if name == "" {
		return entityID
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func (h *DashboardHandler) fetchCalendar(now time.Time) *calendarView {
	ids := h.config.Calendar.EntityIDs
	if len(ids) == 0 {
		return nil
	}

	events, err := h.haClient.GetCalendarEventsForToday(ids)
	if err != nil {
		log.Printf("calendar fetch error: %v", err)
		return nil
	}

	colorByID := make(map[string]template.CSS, len(ids))
	for i, id := range ids {
		colorByID[id] = calendarPalette[i%len(calendarPalette)]
	}

	view := &calendarView{}
	for _, ev := range events {
		entry := eventView{
			Summary:  ev.Summary,
			Location: ev.Location,
			AllDay:   ev.AllDay,
			Source:   shortCalendarLabel(ev.EntityID),
			Color:    colorByID[ev.EntityID],
		}
		if ev.AllDay {
			entry.TimeStr = "All day"
			view.AllDay = append(view.AllDay, entry)
			continue
		}
		entry.TimeStr = ev.Start.Format("15:04")
		entry.Ongoing = !ev.Start.After(now) && ev.End.After(now)
		view.Timed = append(view.Timed, entry)
	}

	if len(view.AllDay) == 0 && len(view.Timed) == 0 {
		return view
	}
	return view
}

func (h *DashboardHandler) fetchWeather() *weatherView {
	entityID := h.config.Weather.EntityID
	if entityID == "" {
		return nil
	}

	wd, err := h.haClient.GetWeather(entityID)
	if err != nil {
		log.Printf("weather fetch error: %v", err)
		return nil
	}

	icon := conditionClass[wd.Condition]
	if icon == "" {
		icon = "wi-cloudy"
	}

	view := &weatherView{
		Temperature: wd.Temperature,
		Condition:   wd.Condition,
		Icon:        icon,
		TempHigh:    wd.TempHigh,
		TempLow:     wd.TempLow,
	}

	if !wd.Sunrise.IsZero() {
		view.SunriseStr = wd.Sunrise.Local().Format("15:04")
	}
	if !wd.Sunset.IsZero() {
		view.SunsetStr = wd.Sunset.Local().Format("15:04")
	}

	for _, hf := range wd.HourlyForecast {
		hIcon := conditionClass[hf.Condition]
		if hIcon == "" {
			hIcon = "wi-cloudy"
		}
		view.HourlyForecast = append(view.HourlyForecast, hourlyForecastView{
			TimeStr:     hf.Time.Local().Format("15"),
			Temperature: hf.Temperature,
			Icon:        hIcon,
		})
	}

	return view
}

// RegisterRoutes registers the dashboard HTML routes on the given mux.
func (h *DashboardHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard", h.handleDashboard)
	mux.HandleFunc("GET /dashboard/partials/clock", h.handleClock)
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

