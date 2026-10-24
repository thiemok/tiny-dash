package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/thiemok/tiny-dash/api/internal/telemetry"
)

// Departure represents a single upcoming departure for a transit line.
type Departure struct {
	Line      string
	Direction string
	Planned   time.Time
	Estimated time.Time
	Cancelled bool
	Alerts    bool
	Transport string
	HeadSign  string
}

// DelayMinutes returns the difference between estimated and planned departure
// in whole minutes (positive = late, 0 = on time, negative shouldn't occur).
func (d Departure) DelayMinutes() int {
	if d.Estimated.IsZero() || d.Planned.IsZero() {
		return 0
	}
	return int(d.Estimated.Sub(d.Planned).Round(time.Minute) / time.Minute)
}

// Leaves is when the departure leaves: estimated, else planned.
func (d Departure) Leaves() time.Time {
	if d.Estimated.IsZero() {
		return d.Planned
	}
	return d.Estimated
}

// ErrNoDepartures is returned when every sensor was read but none lists an
// upcoming departure.
var ErrNoDepartures = errors.New("no upcoming departures")

// time entry as exposed by the Public Transport Departures integration.
type transitTime struct {
	Planned   string `json:"planned"`
	Estimated string `json:"estimated"`
	Cancelled bool   `json:"cancelled"`
	HeadSign  string `json:"head_sign"`
	Alerts    bool   `json:"alerts"`
}

type departuresAttributes struct {
	LineName  string        `json:"line_name"`
	Direction string        `json:"direction"`
	Transport string        `json:"transport"`
	Times     []transitTime `json:"times"`
}

// GetNextDeparturePerSensor returns one row per sensor: the soonest
// upcoming departure for that sensor. Sensors with no upcoming entry are
// skipped. Results are sorted by departure time.
func (c *Client) GetNextDeparturePerSensor(ctx context.Context, entityIDs []string) ([]Departure, error) {
	ctx, span := tracer.Start(ctx, "homeassistant.GetNextDeparturePerSensor")
	defer span.End()

	now := time.Now()
	var picks []Departure

	for _, id := range entityIDs {
		state, err := c.GetState(ctx, id)
		if err != nil {
			slog.WarnContext(ctx, "departures fetch failed", "entity_id", id, "error", err)
			continue
		}

		var attrs departuresAttributes
		if err := json.Unmarshal(state.Attributes, &attrs); err != nil {
			slog.WarnContext(ctx, "departures decode failed", "entity_id", id, "error", err)
			continue
		}

		var next *Departure
		for _, t := range attrs.Times {
			d := Departure{
				Line:      attrs.LineName,
				Direction: attrs.Direction,
				Transport: attrs.Transport,
				HeadSign:  t.HeadSign,
				Cancelled: t.Cancelled,
				Alerts:    t.Alerts,
			}
			if pt, err := time.Parse(time.RFC3339, t.Planned); err == nil {
				d.Planned = pt.Local()
			}
			if et, err := time.Parse(time.RFC3339, t.Estimated); err == nil {
				d.Estimated = et.Local()
			}

			ref := d.Estimated
			if ref.IsZero() {
				ref = d.Planned
			}
			if ref.IsZero() || ref.Before(now) {
				continue
			}
			candidate := d
			next = &candidate
			break // times[] is chronological; first future entry is the next one
		}
		if next != nil {
			picks = append(picks, *next)
		}
	}

	sort.SliceStable(picks, func(i, j int) bool {
		a := picks[i].Estimated
		if a.IsZero() {
			a = picks[i].Planned
		}
		b := picks[j].Estimated
		if b.IsZero() {
			b = picks[j].Planned
		}
		return a.Before(b)
	})

	span.SetAttributes(attribute.Int("homeassistant.departures.count", len(picks)))
	if len(picks) == 0 {
		return nil, telemetry.RecordError(span, fmt.Errorf("no upcoming departures across %d sensors", len(entityIDs)))
	}
	return picks, nil
}

// GetUpcomingDepartures aggregates departures from multiple sensors, filters
// out past entries, sorts by estimated/planned time, and returns the next
// `limit` results. Cancelled departures are kept (caller decides how to show).
func (c *Client) GetUpcomingDepartures(ctx context.Context, entityIDs []string, limit int) ([]Departure, error) {
	ctx, span := tracer.Start(ctx, "homeassistant.GetUpcomingDepartures")
	defer span.End()

	now := time.Now()
	var all []Departure
	read := 0

	for _, id := range entityIDs {
		state, err := c.GetState(ctx, id)
		if err != nil {
			slog.WarnContext(ctx, "departures fetch failed", "entity_id", id, "error", err)
			continue
		}

		var attrs departuresAttributes
		if err := json.Unmarshal(state.Attributes, &attrs); err != nil {
			slog.WarnContext(ctx, "departures decode failed", "entity_id", id, "error", err)
			continue
		}
		read++

		for _, t := range attrs.Times {
			d := Departure{
				Line:      attrs.LineName,
				Direction: attrs.Direction,
				Transport: attrs.Transport,
				HeadSign:  t.HeadSign,
				Cancelled: t.Cancelled,
				Alerts:    t.Alerts,
			}
			if pt, err := time.Parse(time.RFC3339, t.Planned); err == nil {
				d.Planned = pt.Local()
			}
			if et, err := time.Parse(time.RFC3339, t.Estimated); err == nil {
				d.Estimated = et.Local()
			}

			ref := d.Estimated
			if ref.IsZero() {
				ref = d.Planned
			}
			if ref.IsZero() || ref.Before(now) {
				continue
			}
			all = append(all, d)
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		a := all[i].Estimated
		if a.IsZero() {
			a = all[i].Planned
		}
		b := all[j].Estimated
		if b.IsZero() {
			b = all[j].Planned
		}
		return a.Before(b)
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	span.SetAttributes(attribute.Int("homeassistant.departures.count", len(all)))
	if len(all) == 0 {
		if read < len(entityIDs) {
			return nil, telemetry.RecordError(span, fmt.Errorf("%d of %d departure sensors could not be read", len(entityIDs)-read, len(entityIDs)))
		}
		return nil, telemetry.RecordError(span, fmt.Errorf("%w across %d sensors", ErrNoDepartures, len(entityIDs)))
	}
	return all, nil
}
