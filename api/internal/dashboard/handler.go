package dashboard

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/thiemok/tiny-dash/api/internal/config"
	"github.com/thiemok/tiny-dash/api/internal/homeassistant"
	"github.com/thiemok/tiny-dash/api/internal/render"
	"github.com/thiemok/tiny-dash/api/internal/telemetry"
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
	Source   string       // short calendar label
	Color    template.CSS // accent color for the calendar source
	Ink      template.CSS // text color that reads on Color
}

// calendarView holds events for today, grouped for display.
type calendarView struct {
	AllDay []eventView
	Timed  []eventView
}

// departureRow holds a single upcoming departure for the template.
type departureRow struct {
	Line      string
	Direction string
	Soon      bool   // departs within departureSoon
	Time      string // planned departure, "15:04"; Delay is shown next to it
	Delay     int    // minutes late (0 = on time)
	Cancelled bool
	Alerts    bool
}

// departuresView holds the list of upcoming departures.
type departuresView struct {
	Departures []departureRow
}

// forecastSlots is how many hourly forecast entries fit the weather card at a
// legible size.
const forecastSlots = 4

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

// hourCell is one hour of the day-curve clock strip. The 24 cells form a
// noon-peak silhouette (short at night, tall around midday) so the current
// hour is readable at a glance from its position on the curve.
type hourCell struct {
	Height int  // px; follows the day curve
	Fill   int  // 0-100, minute fill of the current hour (set on Now only)
	Past   bool // an elapsed hour (solid)
	Now    bool // the current hour (caret + minute fill)
	QStart bool // first hour of a 6-hour quarter (adds a landmark gap)
}

// Day-curve cell height bounds, in px, tuned for the 800x480 header.
const (
	clockCellMin = 14
	clockCellMax = 46
)

// templateData holds values passed to the dashboard template.
type templateData struct {
	Width        int
	Height       int
	Time         string
	Cells        []hourCell // 24-hour day-curve clock strip
	Weekday      string
	Day          string
	Swatches     []template.CSS
	ShowSwatches bool
	Weather      *weatherView
	Calendar     *calendarView
	Departures   *departuresView
	NextRefresh  string // RFC 3339, see nextRefreshMeta
}

// hourCells builds the 24-hour day-curve strip for the given time. Minute is
// quantized to step, which the clock reports as its next change, so a stale
// frame never shows a misleading sub-block.
func hourCells(now time.Time, step time.Duration) []hourCell {
	hour := now.Hour()
	s := int(step / time.Minute)
	fill := (now.Minute() / s) * s * 100 / 60
	cells := make([]hourCell, 24)
	for i := range cells {
		bump := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/24)
		cells[i] = hourCell{
			Height: clockCellMin + int(bump*float64(clockCellMax-clockCellMin)),
			Past:   i < hour,
			Now:    i == hour,
			QStart: i != 0 && i%6 == 0,
		}
		if i == hour {
			cells[i].Fill = fill
		}
	}
	return cells
}

func (h *DashboardHandler) clockStepAt(now time.Time) time.Duration {
	if quiet, _ := quietHours(now, h.config.Refresh.QuietHours); quiet {
		return quietClockStep
	}
	return clockStep
}

func (h *DashboardHandler) newTemplateData(ctx context.Context, width, height int, palette render.Palette, mock bool) templateData {
	now := time.Now()
	step := h.clockStepAt(now)
	cs := changes{}
	cs.add(now, nextClockStep(now, step), sourceClock)

	swatches := make([]template.CSS, len(palette))
	for i, e := range palette {
		swatches[i] = template.CSS(fmt.Sprintf("rgb(%d,%d,%d)", e.R, e.G, e.B))
	}

	data := templateData{
		Width:    width,
		Height:   height,
		Time:     now.Format("15:04"),
		Cells:    hourCells(now, step),
		Weekday:  strings.ToUpper(now.Format("Monday")),
		Day:      now.Format("2 Jan"),
		Swatches: swatches,
	}

	if mock {
		data.Weather = mockWeather(now)
		data.Calendar = mockCalendar(now, calendarColors(palette))
		data.Departures = departureRows(mockDepartures(now), now, &cs)
	} else {
		data.Weather = h.fetchWeather(ctx, now, &cs)
		data.Calendar = h.fetchCalendar(ctx, now, calendarColors(palette), &cs)
		data.Departures = h.fetchDepartures(ctx, now, &cs)
	}

	data.NextRefresh = h.decideRefresh(ctx, now, cs).Format(time.RFC3339)
	return data
}

// decideRefresh picks the next refresh from the sections' changes and
// records the decision on the request's span and the delay metric.
func (h *DashboardHandler) decideRefresh(ctx context.Context, now time.Time, cs changes) time.Time {
	d := decideRefresh(now, h.config.Refresh, cs)
	delay := d.At.Sub(now)
	attrs := []attribute.KeyValue{attrRefreshSource.String(d.Source), attrRefreshQuiet.Bool(d.Quiet)}
	trace.SpanFromContext(ctx).SetAttributes(append(attrs, attrRefreshDelay.Int64(int64(delay/time.Second)))...)
	refreshDelay.Record(ctx, delay.Seconds(), metric.WithAttributes(attrs...))
	slog.InfoContext(ctx, "next refresh decided", "delay", delay, "source", d.Source, "quiet", d.Quiet)
	return d.At
}

func (h *DashboardHandler) sectionFailed(now time.Time, cs *changes, source string) {
	cs.add(now, now.Add(h.config.Refresh.Retry), source)
}

// departureTime formats the planned departure in now's zone. A clock time,
// unlike "in 5 min", stays valid until the departure drops off the list, so
// the panel need not be refreshed every minute.
func departureTime(d homeassistant.Departure, now time.Time) string {
	t := d.Planned
	if t.IsZero() {
		t = d.Estimated
	}
	return t.In(now.Location()).Format("15:04")
}

// startFetch opens the span for one dashboard data source fetch.
func startFetch(ctx context.Context, source string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "dashboard.fetch "+source, trace.WithAttributes(attrDataSource.String(source)))
}

// fetchFailed records a data source failure on the span, the failure counter and the log.
func fetchFailed(ctx context.Context, span trace.Span, source string, err error) {
	telemetry.RecordError(span, err)
	dataSourceFailures.Add(ctx, 1, metric.WithAttributes(attrDataSource.String(source)))
	slog.WarnContext(ctx, "dashboard data source fetch failed", "datasource", source, "error", err)
}

func (h *DashboardHandler) fetchDepartures(ctx context.Context, now time.Time, cs *changes) *departuresView {
	ids := h.config.Departures.EntityIDs
	if len(ids) == 0 {
		return nil
	}

	ctx, span := startFetch(ctx, "departures")
	defer span.End()

	departures, err := h.haClient.GetUpcomingDepartures(ctx, ids, 0)
	if err != nil {
		fetchFailed(ctx, span, "departures", err)
		// An empty board is current data; only a failed read is worth retrying.
		if !errors.Is(err, homeassistant.ErrNoDepartures) {
			h.sectionFailed(now, cs, sourceDepartures)
		}
		return nil
	}
	return departureRows(departures, now, cs)
}

// departureRows builds the departures view. A row changes when it turns
// red departureSoon before leaving, and when it leaves and drops off the
// list.
func departureRows(departures []homeassistant.Departure, now time.Time, cs *changes) *departuresView {
	view := &departuresView{}
	for _, d := range departures {
		leaves := d.Leaves()
		row := departureRow{
			Line:      d.Line,
			Direction: d.Direction,
			Soon:      leaves.Sub(now) < departureSoon,
			Time:      departureTime(d, now),
			Delay:     d.DelayMinutes(),
			Cancelled: d.Cancelled,
			Alerts:    d.Alerts,
		}
		view.Departures = append(view.Departures, row)
		cs.add(now, leaves.Add(-departureSoon), sourceDepartures)
		cs.add(now, leaves, sourceDepartures)
	}
	return view
}

// calendarColor is a source calendar's accent and the text color that
// reads on top of it (all-day events render as filled bars).
type calendarColor struct {
	Index byte // panel color index of Fill
	Fill  template.CSS
	Ink   template.CSS
}

// calendarPalette assigns an accent color per source calendar (cycled).
// Only exact panel colors (see designColors), so the image needs no dithering.
var calendarPalette = []calendarColor{
	{5, "#0000ff", "#ffffff"}, // blue
	{3, "#ff0000", "#ffffff"}, // red
	{6, "#00ff00", "#000000"}, // green
	{2, "#ffff00", "#000000"}, // yellow
}

// calendarColors returns the calendar accents the display can show. A color
// it lacks would be dithered into something else (green turns black on a
// black/white/yellow/red panel), hiding the text drawn on it.
func calendarColors(palette render.Palette) []calendarColor {
	var colors []calendarColor
	for _, c := range calendarPalette {
		if palette.Covers([]byte{c.Index}) {
			colors = append(colors, c)
		}
	}
	if len(colors) == 0 {
		return []calendarColor{{0, "#000000", "#ffffff"}}
	}
	return colors
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

func (h *DashboardHandler) fetchCalendar(ctx context.Context, now time.Time, colors []calendarColor, cs *changes) *calendarView {
	ids := h.config.Calendar.EntityIDs
	if len(ids) == 0 {
		return nil
	}

	ctx, span := startFetch(ctx, "calendar")
	defer span.End()

	events, err := h.haClient.GetCalendarEventsForToday(ctx, ids)
	if err != nil {
		fetchFailed(ctx, span, "calendar", err)
		h.sectionFailed(now, cs, sourceCalendar)
		return nil
	}

	colorByID := make(map[string]calendarColor, len(ids))
	for i, id := range ids {
		colorByID[id] = colors[i%len(colors)]
	}

	view := &calendarView{}
	for _, ev := range events {
		entry := eventView{
			Summary:  ev.Summary,
			Location: ev.Location,
			AllDay:   ev.AllDay,
			Source:   shortCalendarLabel(ev.EntityID),
			Color:    colorByID[ev.EntityID].Fill,
			Ink:      colorByID[ev.EntityID].Ink,
		}
		if ev.AllDay {
			entry.TimeStr = "All day"
			view.AllDay = append(view.AllDay, entry)
			continue
		}
		// Finished events only take space from the ones still ahead.
		if !ev.End.After(now) {
			continue
		}
		entry.TimeStr = ev.Start.Format("15:04")
		entry.Ongoing = !ev.Start.After(now) && ev.End.After(now)
		view.Timed = append(view.Timed, entry)
		// It turns ongoing at its start and drops off at its end.
		cs.add(now, ev.Start, sourceCalendar)
		cs.add(now, ev.End, sourceCalendar)
	}

	if len(view.AllDay) == 0 && len(view.Timed) == 0 {
		return view
	}
	return view
}

func (h *DashboardHandler) fetchWeather(ctx context.Context, now time.Time, cs *changes) *weatherView {
	entityID := h.config.Weather.EntityID
	if entityID == "" {
		return nil
	}

	ctx, span := startFetch(ctx, "weather")
	defer span.End()

	wd, err := h.haClient.GetWeather(ctx, entityID)
	if err != nil {
		fetchFailed(ctx, span, "weather", err)
		h.sectionFailed(now, cs, sourceWeather)
		return nil
	}
	// The hourly slots move on every hour; sunrise and sunset move on to the
	// next day once they have passed.
	cs.add(now, nextClockStep(now, time.Hour), sourceWeather)
	cs.add(now, wd.Sunrise, sourceWeather)
	cs.add(now, wd.Sunset, sourceWeather)

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
		if len(view.HourlyForecast) == forecastSlots {
			break
		}
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
	mock := r.URL.Query().Get("mock") == "1"
	data := h.newTemplateData(r.Context(), width, height, palette, mock)
	data.ShowSwatches = r.URL.Query().Get("swatches") == "1"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		telemetry.RecordError(trace.SpanFromContext(r.Context()), err)
		slog.ErrorContext(r.Context(), "dashboard template failed", "error", err)
	}
}

func (h *DashboardHandler) handleClock(w http.ResponseWriter, r *http.Request) {
	// The clock partial refreshes every 10s; only the strip is needed, so skip
	// the Home Assistant fetches that newTemplateData would otherwise perform.
	now := time.Now()
	data := templateData{Cells: hourCells(now, h.clockStepAt(now))}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "clock", data); err != nil {
		telemetry.RecordError(trace.SpanFromContext(r.Context()), err)
		slog.ErrorContext(r.Context(), "clock template failed", "error", err)
	}
}
