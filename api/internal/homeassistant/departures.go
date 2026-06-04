package homeassistant

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"
)

// Departure represents a single upcoming departure for a transit line.
type Departure struct {
	Line        string
	Direction   string
	Planned     time.Time
	Estimated   time.Time
	Cancelled   bool
	Alerts      bool
	Transport   string
	HeadSign    string
}

// DelayMinutes returns the difference between estimated and planned departure
// in whole minutes (positive = late, 0 = on time, negative shouldn't occur).
func (d Departure) DelayMinutes() int {
	if d.Estimated.IsZero() || d.Planned.IsZero() {
		return 0
	}
	return int(d.Estimated.Sub(d.Planned).Round(time.Minute) / time.Minute)
}

// MinutesUntil returns the whole minutes from now until the (estimated)
// departure. Negative if already past.
func (d Departure) MinutesUntil(now time.Time) int {
	t := d.Estimated
	if t.IsZero() {
		t = d.Planned
	}
	return int(t.Sub(now).Round(time.Minute) / time.Minute)
}

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

// GetUpcomingDepartures aggregates departures from multiple sensors, filters
// out past entries, sorts by estimated/planned time, and returns the next
// `limit` results. Cancelled departures are kept (caller decides how to show).
func (c *Client) GetUpcomingDepartures(entityIDs []string, limit int) ([]Departure, error) {
	now := time.Now()
	var all []Departure

	for _, id := range entityIDs {
		state, err := c.GetState(id)
		if err != nil {
			log.Printf("departures fetch error for %s: %v", id, err)
			continue
		}

		var attrs departuresAttributes
		if err := json.Unmarshal(state.Attributes, &attrs); err != nil {
			log.Printf("departures decode error for %s: %v", id, err)
			continue
		}

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
	if len(all) == 0 {
		return nil, fmt.Errorf("no upcoming departures across %d sensors", len(entityIDs))
	}
	return all, nil
}
