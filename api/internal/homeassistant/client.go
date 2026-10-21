package homeassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/thiemok/tiny-dash/api/internal/telemetry"
)

var tracer = otel.Tracer("github.com/thiemok/tiny-dash/api/internal/homeassistant")

// attrEntityID identifies the Home Assistant entity an operation targets.
// Entity IDs come from config, so cardinality is bounded.
var attrEntityID = attribute.Key("homeassistant.entity_id")

// Client communicates with a Home Assistant instance via its REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Home Assistant API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// GetState fetches the current state of a single entity.
func (c *Client) GetState(ctx context.Context, entityID string) (*State, error) {
	ctx, span := tracer.Start(ctx, "homeassistant.GetState", trace.WithAttributes(attrEntityID.String(entityID)))
	defer span.End()

	path := fmt.Sprintf("/api/states/%s", url.PathEscape(entityID))
	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("getting state for %s: %w", entityID, err))
	}

	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("decoding state for %s: %w", entityID, err))
	}
	return &state, nil
}

// CallService invokes a Home Assistant service and returns the response.
func (c *Client) CallService(ctx context.Context, domain, service string, data any) (ServiceResponse, error) {
	ctx, span := tracer.Start(ctx, "homeassistant.CallService", trace.WithAttributes(
		attribute.String("homeassistant.service", domain+"."+service),
	))
	defer span.End()

	path := fmt.Sprintf("/api/services/%s/%s?return_response", url.PathEscape(domain), url.PathEscape(service))

	var reqBody io.Reader
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, telemetry.RecordError(span, fmt.Errorf("encoding service call data: %w", err))
		}
		reqBody = bytes.NewReader(b)
	}

	body, err := c.doRequest(ctx, http.MethodPost, path, reqBody)
	if err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("calling service %s/%s: %w", domain, service, err))
	}

	var result serviceCallResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("decoding service response for %s/%s: %w", domain, service, err))
	}
	return result.ServiceResponse, nil
}

// GetCalendarEvents fetches events from a calendar entity for the given time range.
func (c *Client) GetCalendarEvents(ctx context.Context, entityID string, start, end time.Time) ([]CalendarEvent, error) {
	ctx, span := tracer.Start(ctx, "homeassistant.GetCalendarEvents", trace.WithAttributes(attrEntityID.String(entityID)))
	defer span.End()

	path := fmt.Sprintf("/api/calendars/%s?start=%s&end=%s",
		url.PathEscape(entityID),
		url.QueryEscape(start.Format(time.RFC3339)),
		url.QueryEscape(end.Format(time.RFC3339)),
	)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("getting calendar events for %s: %w", entityID, err))
	}

	var events []CalendarEvent
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("decoding calendar events for %s: %w", entityID, err))
	}
	span.SetAttributes(attribute.Int("homeassistant.calendar.event_count", len(events)))
	return events, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
