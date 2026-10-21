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
)

var (
	dataSourceFailures = instrument(meter.Int64Counter("tinydash.datasource.failures",
		metric.WithDescription("Dashboard data source fetches that failed and left their section empty."),
		metric.WithUnit("{failure}"),
	))
	renderDuration = instrument(meter.Float64Histogram("tinydash.render.duration",
		metric.WithDescription("Time to produce a dashboard image: Chrome capture, dithering and encoding."),
		metric.WithUnit("s"),
	))
)

func instrument[T any](v T, err error) T {
	if err != nil {
		otel.Handle(err)
	}
	return v
}
