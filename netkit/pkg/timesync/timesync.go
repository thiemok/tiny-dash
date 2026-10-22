// Package timesync decides when a device syncs its clock with a time server:
// right away, then every Interval after a success, and after a failure with
// capped exponential backoff, so an unreachable server is retried forever
// without being hammered. It does no I/O and does not allocate.
package timesync

import (
	"time"

	"github.com/thiemok/tiny-dash/netkit/pkg/backoff"
)

// Policy controls when syncs are due.
type Policy struct {
	// Interval is the time from a successful sync to the next one.
	Interval time.Duration
	// Retry spaces the attempts after a failed sync.
	Retry backoff.Exponential
}

// Schedule tracks when the next sync is due. Create one with New.
type Schedule struct {
	policy Policy
	next   time.Time
	// due is set until the first sync and by Invalidate, independent of the
	// clock that next is compared with.
	due bool
}

// New returns a Schedule whose first sync is due at once.
func New(policy Policy) *Schedule {
	s := &Schedule{policy: policy, due: true}
	s.policy.Retry.Reset()
	return s
}

// Due reports whether a sync is due at now.
func (s *Schedule) Due(now time.Time) bool {
	return s.due || !now.Before(s.next)
}

// Next returns when the next sync is due, or the zero time if it is due at
// once.
func (s *Schedule) Next() time.Time {
	if s.due {
		return time.Time{}
	}
	return s.next
}

// Failures reports how many syncs failed in a row.
func (s *Schedule) Failures() uint { return s.policy.Retry.Attempts() }

// Succeeded records a successful sync at now.
func (s *Schedule) Succeeded(now time.Time) {
	s.policy.Retry.Reset()
	s.due = false
	s.next = now.Add(s.policy.Interval)
}

// Failed records a failed sync at now and returns the delay until the next
// attempt; rnd jitters it as in backoff.Exponential.Next.
func (s *Schedule) Failed(now time.Time, rnd uint32) time.Duration {
	wait := s.policy.Retry.Next(rnd)
	s.due = false
	s.next = now.Add(wait)
	return wait
}

// Invalidate makes a sync due at once, e.g. after a network reconnect. The
// failure count is kept, so a server that still fails is retried no more
// often than before.
func (s *Schedule) Invalidate() { s.due = true }
