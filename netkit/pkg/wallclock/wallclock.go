// Package wallclock keeps wall time for devices that boot without one: a
// monotonic clock plus an offset learned from a time server, e.g. over NTP.
//
// Until the first Set the clock is not synced: Now and UnixNano report false
// and Nanotime returns 0, so a certificate check fed by it fails closed
// instead of trusting a time counted from boot. Times before a floor, such as
// the firmware's build time, are refused, so a bogus server cannot move the
// clock back past it.
//
// Reading the clock does not allocate, and it is safe to read from any
// goroutine while another one sets it.
package wallclock

import (
	"errors"
	"sync/atomic"
	"time"
)

var ErrBeforeFloor = errors.New("wallclock: time is before the floor")

// Clock is a wall clock. Create one with New.
type Clock struct {
	mono  func() int64
	floor int64

	// offset is wall minus mono in nanoseconds; it is 0 until the first Set.
	offset atomic.Int64
	synced atomic.Bool
}

// New returns an unsynced Clock. mono returns monotonic nanoseconds, from
// any base; it must never go backwards. Set refuses times before floor.
func New(mono func() int64, floor time.Time) *Clock {
	return &Clock{mono: mono, floor: floor.UnixNano()}
}

// Floor returns the earliest time Set accepts.
func (c *Clock) Floor() time.Time { return time.Unix(0, c.floor).UTC() }

// Synced reports whether the clock has been set.
func (c *Clock) Synced() bool { return c.synced.Load() }

// Set makes wall the time at the monotonic instant mono, e.g. the moment a
// time server's response arrived, and syncs the clock.
//
// step is how far the clock is moved: positive if it was behind. Before the
// first Set the clock reads mono itself, so the first step is the wall time
// at mono's base, e.g. at boot.
func (c *Clock) Set(wall time.Time, mono int64) (step time.Duration, err error) {
	w := wall.UnixNano()
	if w < c.floor {
		return 0, ErrBeforeFloor
	}
	offset := w - mono
	step = time.Duration(offset - c.offset.Swap(offset))
	c.synced.Store(true)
	return step, nil
}

// UnixNano returns the wall time in Unix nanoseconds, and false while the
// clock is not synced.
func (c *Clock) UnixNano() (int64, bool) {
	if !c.synced.Load() {
		return 0, false
	}
	return c.mono() + c.offset.Load(), true
}

// Now returns the wall time in UTC, and false while the clock is not synced.
func (c *Clock) Now() (time.Time, bool) {
	ns, ok := c.UnixNano()
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(0, ns).UTC(), true
}

// Nanotime returns the wall time in Unix nanoseconds, or 0 while the clock is
// not synced. It fits a certificate verifier's clock hook: at 0 every
// certificate is not yet valid, so a caller that does not check Synced first
// still fails closed.
func (c *Clock) Nanotime() int64 {
	ns, _ := c.UnixNano()
	return ns
}
