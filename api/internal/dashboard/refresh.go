package dashboard

import (
	"time"

	"github.com/thiemok/tiny-dash/api/internal/config"
)

// Each dashboard section reports when its rendering changes next. The page
// carries the earliest of those, bounded by the refresh config, in a single
// meta tag (nextRefreshMeta), which the image handler turns into the
// device's X-Refresh-After header.

// nextRefreshMeta names the meta tag holding the next refresh time (RFC 3339).
const nextRefreshMeta = "tinydash-next-refresh"

// refreshHintHeader carries the delay until the next refresh, in whole
// seconds, to the device (netkit/pkg/refresh.HintHeader).
const refreshHintHeader = "X-Refresh-After"

// Clock strip steps: the fill moves every clockStep, and every hour during
// quiet hours, so the clock does not wake the panel all night.
const (
	clockStep      = 5 * time.Minute
	quietClockStep = time.Hour
)

// departureSoon is how long before departing a row turns red.
const departureSoon = 5 * time.Minute

// Sources of a refresh decision, bounded for telemetry: a section name, or
// the bound that overrode the sections.
const (
	sourceClock      = "clock"
	sourceCalendar   = "calendar"
	sourceDepartures = "departures"
	sourceWeather    = "weather"
	sourceQuietEnd   = "quiet_end"
	sourceMin        = "min"
	sourceMax        = "max"
)

// change is when a section's rendering changes next.
type change struct {
	At     time.Time
	Source string
}

// changes collects the sections' next changes during one render.
type changes []change

// add records a change at at, ignoring times that are not in the future.
func (c *changes) add(now, at time.Time, source string) {
	if at.After(now) {
		*c = append(*c, change{at, source})
	}
}

// refreshDecision is when the device should fetch next, and why.
type refreshDecision struct {
	At     time.Time
	Source string
	Quiet  bool
}

// decideRefresh picks the earliest change, bounded by cfg's min and max (or
// the quiet hours' while they last). The end of quiet hours caps the delay,
// so the daytime cadence resumes on time.
func decideRefresh(now time.Time, cfg config.RefreshConfig, cs changes) refreshDecision {
	minD, maxD, maxSource := cfg.Min, cfg.Max, sourceMax
	quiet, quietEnd := quietHours(now, cfg.QuietHours)
	if quiet {
		minD, maxD = cfg.QuietHours.Min, cfg.QuietHours.Max
		if untilEnd := quietEnd.Sub(now); untilEnd <= maxD {
			maxD, maxSource = untilEnd, sourceQuietEnd
			minD = min(minD, maxD)
		}
	}

	d := refreshDecision{At: now.Add(maxD), Source: maxSource, Quiet: quiet}
	for _, c := range cs {
		if c.At.Before(d.At) {
			d.At, d.Source = c.At, c.Source
		}
	}
	if earliest := now.Add(minD); d.At.Before(earliest) {
		d.At, d.Source = earliest, sourceMin
		if minD == maxD {
			d.Source = maxSource
		}
	}
	return d
}

// quietHours reports whether now falls in q's window and, if so, when the
// window ends.
func quietHours(now time.Time, q config.QuietHoursConfig) (bool, time.Time) {
	if !q.Enabled() {
		return false, time.Time{}
	}
	start, end := clockMinutes(q.Start), clockMinutes(q.End)
	m := now.Hour()*60 + now.Minute()
	var in bool
	if start <= end {
		in = start <= m && m < end
	} else { // crosses midnight
		in = m >= start || m < end
	}
	if !in {
		return false, time.Time{}
	}
	endAt := atClockMinutes(now, end)
	if !endAt.After(now) {
		endAt = atClockMinutes(now.AddDate(0, 0, 1), end)
	}
	return true, endAt
}

// clockMinutes parses a validated "15:04" into minutes after midnight.
func clockMinutes(hhmm string) int {
	t, _ := time.Parse("15:04", hhmm)
	return t.Hour()*60 + t.Minute()
}

// atClockMinutes is m minutes after midnight on day's date, in day's zone.
// time.Date normalizes wall times that a DST switch skips.
func atClockMinutes(day time.Time, m int) time.Time {
	y, mo, d := day.Date()
	return time.Date(y, mo, d, 0, m, 0, 0, day.Location())
}

// nextClockStep is the next wall-clock multiple of step after now, counted
// from midnight so steps stay on the hour in any time zone.
func nextClockStep(now time.Time, step time.Duration) time.Time {
	s := int(step / time.Minute)
	m := now.Hour()*60 + now.Minute()
	return atClockMinutes(now, (m/s+1)*s)
}
