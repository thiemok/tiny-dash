package main

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	api "github.com/thiemok/tiny-dash/api"
	"github.com/thiemok/tiny-dash/api/internal/config"
	"github.com/thiemok/tiny-dash/api/internal/dashboard"
	"github.com/thiemok/tiny-dash/api/internal/homeassistant"
)

var spans = tracetest.NewSpanRecorder()

func TestMain(m *testing.M) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	os.Exit(m.Run())
}

func newTestServer(t *testing.T, haURL string, cfg *config.Config) *httptest.Server {
	t.Helper()
	spans.Reset()
	dashHandler, err := dashboard.NewDashboardHandler(api.TemplateFS, homeassistant.NewClient(haURL, "token"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newHandler(dashHandler, dashboard.NewImageHandler(nil, "")))
	t.Cleanup(srv.Close)
	return srv
}

// get performs a request and then closes srv, which waits for the handler
// (and so the server span) to finish before spans are inspected.
func get(t *testing.T, srv *httptest.Server, path string, header http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	maps.Copy(req.Header, header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	srv.Close()
}

func findSpan(t *testing.T, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range spans.Ended() {
		if s.Name() == name {
			return s
		}
	}
	var names []string
	for _, s := range spans.Ended() {
		names = append(names, s.Name())
	}
	t.Fatalf("no span %q; got %v", name, names)
	return nil
}

func attr(s sdktrace.ReadOnlySpan, key attribute.Key) attribute.Value {
	for _, kv := range s.Attributes() {
		if kv.Key == key {
			return kv.Value
		}
	}
	return attribute.Value{}
}

func TestServerSpansUseRoutePattern(t *testing.T) {
	srv := newTestServer(t, "http://unused", &config.Config{})

	// Chrome loads /dashboard with the image request's traceparent; it must continue that trace.
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	get(t, srv, "/dashboard?mock=1", http.Header{
		"Traceparent": {"00-" + traceID + "-00f067aa0ba902b7-01"},
	})

	s := findSpan(t, "GET /dashboard")
	if got := attr(s, semconv.HTTPRouteKey).AsString(); got != "/dashboard" {
		t.Errorf("http.route = %q, want /dashboard", got)
	}
	if got := s.SpanContext().TraceID().String(); got != traceID {
		t.Errorf("trace id = %s, want propagated %s", got, traceID)
	}
}

func TestHealthCheckIsNotTraced(t *testing.T) {
	srv := newTestServer(t, "http://unused", &config.Config{})

	get(t, srv, healthPath, nil)

	if n := len(spans.Ended()); n != 0 {
		t.Errorf("health check produced %d spans, want 0", n)
	}
}

func TestHomeAssistantFailureIsRecordedAndPropagated(t *testing.T) {
	var gotTraceparent string
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("Traceparent")
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ha.Close()

	srv := newTestServer(t, ha.URL, &config.Config{Weather: config.WeatherConfig{EntityID: "weather.home"}})
	get(t, srv, "/dashboard", nil)

	server := findSpan(t, "GET /dashboard")
	fetch := findSpan(t, "dashboard.fetch weather")
	if fetch.Status().Code != codes.Error {
		t.Errorf("fetch span status = %v, want Error", fetch.Status().Code)
	}
	if fetch.Parent().SpanID() != server.SpanContext().SpanID() {
		t.Error("fetch span is not a child of the server span")
	}
	if findSpan(t, "homeassistant.GetWeather").Status().Code != codes.Error {
		t.Error("homeassistant.GetWeather span not marked as error")
	}
	client := findSpan(t, "HTTP GET")
	if gotTraceparent == "" || client.SpanContext().TraceID() != server.SpanContext().TraceID() {
		t.Errorf("outbound HA request not propagated in trace (traceparent %q)", gotTraceparent)
	}
}
