package homeassistant

import "encoding/json"

// State represents a Home Assistant entity state response.
type State struct {
	EntityID    string            `json:"entity_id"`
	State       string            `json:"state"`
	Attributes  json.RawMessage   `json:"attributes"`
	LastChanged string            `json:"last_changed"`
	LastUpdated string            `json:"last_updated"`
}

// CalendarEvent represents a single event from the HA calendar API.
type CalendarEvent struct {
	Summary     string `json:"summary"`
	Start       Time   `json:"start"`
	End         Time   `json:"end"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
}

// Time handles HA calendar time fields which can be either a date string
// (for all-day events) or a dateTime string (for timed events).
type Time struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
}

// IsAllDay returns true if this time represents an all-day event.
func (t Time) IsAllDay() bool {
	return t.Date != "" && t.DateTime == ""
}

// serviceCallResult is the top-level response envelope from a HA service call.
type serviceCallResult struct {
	ServiceResponse ServiceResponse `json:"service_response"`
}

// ServiceResponse represents the inner service_response from a HA service call.
type ServiceResponse map[string]json.RawMessage
