package homeassistant

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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
			Timeout: 10 * time.Second,
		},
	}
}

// GetState fetches the current state of a single entity.
func (c *Client) GetState(entityID string) (*State, error) {
	path := fmt.Sprintf("/api/states/%s", url.PathEscape(entityID))
	body, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting state for %s: %w", entityID, err)
	}

	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("decoding state for %s: %w", entityID, err)
	}
	return &state, nil
}

// CallService invokes a Home Assistant service and returns the response.
func (c *Client) CallService(domain, service string, data any) (ServiceResponse, error) {
	path := fmt.Sprintf("/api/services/%s/%s", url.PathEscape(domain), url.PathEscape(service))

	var reqBody io.Reader
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("encoding service call data: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	body, err := c.doRequest(http.MethodPost, path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("calling service %s/%s: %w", domain, service, err)
	}

	var resp ServiceResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding service response for %s/%s: %w", domain, service, err)
	}
	return resp, nil
}

// GetCalendarEvents fetches events from a calendar entity for the given time range.
func (c *Client) GetCalendarEvents(entityID string, start, end time.Time) ([]CalendarEvent, error) {
	path := fmt.Sprintf("/api/calendars/%s?start=%s&end=%s",
		url.PathEscape(entityID),
		url.QueryEscape(start.Format(time.RFC3339)),
		url.QueryEscape(end.Format(time.RFC3339)),
	)

	body, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting calendar events for %s: %w", entityID, err)
	}

	var events []CalendarEvent
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("decoding calendar events for %s: %w", entityID, err)
	}
	return events, nil
}

func (c *Client) doRequest(method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
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
