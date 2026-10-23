package dashboard

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const scopeName = "github.com/thiemok/tiny-dash/api/internal/dashboard"

var (
	tracer = otel.Tracer(scopeName)
	meter  = otel.Meter(scopeName)
)

var (
	// attrDataSource names the dashboard section a fetch belongs to: weather, calendar or departures.
	attrDataSource = attribute.Key("tinydash.datasource")
	// attrRenderOutput is the image format produced by a render: packed or png.
	attrRenderOutput = attribute.Key("tinydash.render.output")
	// attrRenderDither is whether a render was dithered (true) or snapped to the nearest palette colors (false).
	attrRenderDither = attribute.Key("tinydash.render.dither")
	// attrRefreshSource is what set the next refresh: a section (clock, calendar, departures,
	// weather), the end of quiet hours (quiet_end), or a bound (min, max).
	attrRefreshSource = attribute.Key("tinydash.refresh.source")
	// attrRefreshQuiet is whether the refresh was decided during quiet hours.
	attrRefreshQuiet = attribute.Key("tinydash.refresh.quiet")
	// attrRefreshDelay is the decided delay until the next refresh, in seconds (span only).
	attrRefreshDelay = attribute.Key("tinydash.refresh.delay_s")
)

var (
	dataSourceFailures = instrument(meter.Int64Counter("tinydash.datasource.failures",
		metric.WithDescription("Dashboard data source fetches that failed and left their section empty."),
		metric.WithUnit("{failure}"),
	))
	renderDuration = instrument(meter.Float64Histogram("tinydash.render.duration",
		metric.WithDescription("Time to produce a dashboard image: Chrome capture, palette reduction and encoding."),
		metric.WithUnit("s"),
	))
	refreshDelay = instrument(meter.Float64Histogram("tinydash.refresh.delay",
		metric.WithDescription("Delay until the next device refresh, decided per dashboard render."),
		metric.WithUnit("s"),
	))
)

func instrument[T any](v T, err error) T {
	if err != nil {
		otel.Handle(err)
	}
	return v
}
