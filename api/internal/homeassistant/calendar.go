package homeassistant

import (
	"log"
	"sort"
	"time"
)

// AggregatedEvent enriches a CalendarEvent with its source entity ID and
// parsed start/end times to simplify rendering.
type AggregatedEvent struct {
	EntityID string
	Summary  string
	Location string
	Start    time.Time
	End      time.Time
	AllDay   bool
}

// GetCalendarEventsForToday fetches events from each given entity covering
// the current local day, merges them, and returns them sorted by start time.
// All-day events appear before timed events.
func (c *Client) GetCalendarEventsForToday(entityIDs []string) ([]AggregatedEvent, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var all []AggregatedEvent
	for _, id := range entityIDs {
		events, err := c.GetCalendarEvents(id, startOfDay, endOfDay)
		if err != nil {
			log.Printf("calendar fetch error for %s: %v", id, err)
			continue
		}
		for _, ev := range events {
			ae := AggregatedEvent{
				EntityID: id,
				Summary:  ev.Summary,
				Location: ev.Location,
				AllDay:   ev.Start.IsAllDay(),
			}
			if ae.AllDay {
				if t, err := time.ParseInLocation("2006-01-02", ev.Start.Date, now.Location()); err == nil {
					ae.Start = t
				}
				if t, err := time.ParseInLocation("2006-01-02", ev.End.Date, now.Location()); err == nil {
					ae.End = t
				}
			} else {
				if t, err := time.Parse(time.RFC3339, ev.Start.DateTime); err == nil {
					ae.Start = t.Local()
				}
				if t, err := time.Parse(time.RFC3339, ev.End.DateTime); err == nil {
					ae.End = t.Local()
				}
			}
			all = append(all, ae)
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].AllDay != all[j].AllDay {
			return all[i].AllDay
		}
		return all[i].Start.Before(all[j].Start)
	})

	return all, nil
}
