// Package refresh decides when a device fetches its next dashboard image: at
// the delay the server asks for, clamped to bounds that protect the panel and
// keep the image from going stale, or at a fixed default when the server
// gives none. A failed fetch is retried with capped exponential backoff,
// independent of the server's delay, and a button press brings the next fetch
// forward. It does no I/O and does not allocate.
package refresh

import (
	"time"

	"github.com/thiemok/tiny-dash/netkit/pkg/backoff"
)

// HintHeader is the response header that carries the server's delay until the
// next fetch, in whole seconds.
const HintHeader = "X-Refresh-After"

// Policy controls when fetches are due.
type Policy struct {
	// Default is the delay after a successful fetch whose response carried no
	// usable hint.
	Default time.Duration
	// Min and Max clamp the server's hint.
	Min, Max time.Duration
	// PressMin is the earliest a button press forces a fetch after the
	// previous one, so the panel is never refreshed back to back.
	PressMin time.Duration
	// Retry spaces the attempts after a failed fetch.
	Retry backoff.Exponential
}

// Source names where a delay came from, for logging.
type Source uint8

const (
	SourceDefault Source = iota
	SourceHint
	SourceRetry
)

func (s Source) String() string {
	switch s {
	case SourceHint:
		return "hint"
	case SourceRetry:
		return "retry"
	default:
		return "default"
	}
}

// Schedule tracks when the next fetch is due. Create one with New.
type Schedule struct {
	policy Policy
	// last is when the previous fetch got its response, the zero time if
	// none did yet.
	last time.Time
	next time.Time
}

// New returns a Schedule whose first fetch is due at once.
func New(policy Policy) *Schedule {
	s := &Schedule{policy: policy}
	s.policy.Retry.Reset()
	return s
}

// Next returns when the next fetch is due.
func (s *Schedule) Next() time.Time { return s.next }

// Succeeded records a successful fetch whose response arrived at at and
// returns the delay from at until the next one. hint is the server's delay,
// ok false when the response carried none.
func (s *Schedule) Succeeded(at time.Time, hint time.Duration, ok bool) (time.Duration, Source) {
	s.policy.Retry.Reset()
	s.last = at
	d, src := s.policy.Default, SourceDefault
	if ok {
		d, src = min(max(hint, s.policy.Min), s.policy.Max), SourceHint
	}
	s.next = at.Add(d)
	return d, src
}

// Failed records a failed fetch at now and returns the delay until the next
// attempt; rnd jitters it as in backoff.Exponential.Next.
func (s *Schedule) Failed(now time.Time, rnd uint32) time.Duration {
	d := s.policy.Retry.Next(rnd)
	s.next = now.Add(d)
	return d
}

// Failures reports how many fetches failed in a row.
func (s *Schedule) Failures() uint { return s.policy.Retry.Attempts() }

// Press records a button press at now and returns when the forced fetch is
// due: at once, or PressMin after the previous fetch if that is later. It
// never postpones a fetch that is due earlier anyway.
func (s *Schedule) Press(now time.Time) time.Time {
	due := now
	if !s.last.IsZero() {
		if earliest := s.last.Add(s.policy.PressMin); earliest.After(due) {
			due = earliest
		}
	}
	if s.next.IsZero() || due.Before(s.next) {
		s.next = due
	}
	return s.next
}

// ParseHint parses a HintHeader value: whole seconds, optionally surrounded
// by spaces. ok is false for an empty, malformed or overflowing value, which
// the caller treats as no hint.
func ParseHint(b []byte) (d time.Duration, ok bool) {
	for len(b) > 0 && b[0] == ' ' {
		b = b[1:]
	}
	for len(b) > 0 && b[len(b)-1] == ' ' {
		b = b[:len(b)-1]
	}
	// Nine digits are three decades, far beyond any clamp; ten could overflow
	// time.Duration.
	if len(b) == 0 || len(b) > 9 {
		return 0, false
	}
	var secs int64
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		secs = secs*10 + int64(c-'0')
	}
	return time.Duration(secs) * time.Second, true
}

// ETagLen is the length of the API's image ETag in hex digits, without the
// quotes. Its fixed width lets the device keep the last one in a slot of its
// prebuilt request.
const ETagLen = 16

// NoETag fills the request's ETag slot while the device has no image to keep:
// not hex, so it never matches, and the server always sends the image.
const NoETag = "----------------"

// ParseETag copies the ETag header value b, a quoted string of ETagLen hex
// digits, into dst without the quotes. ok is false for any other value,
// which the caller treats as no ETag.
func ParseETag(b []byte, dst *[ETagLen]byte) (ok bool) {
	if len(b) != ETagLen+2 || b[0] != '"' || b[len(b)-1] != '"' {
		return false
	}
	for _, c := range b[1 : len(b)-1] {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	copy(dst[:], b[1:len(b)-1])
	return true
}
