// Package backoff computes capped exponential retry delays with jitter.
// It does not allocate, so it is usable in TinyGo firmware retry loops.
package backoff

import (
	"math/bits"
	"time"
)

// Exponential doubles the delay after every attempt, starting at Min and
// capped at Max. The zero value retries immediately; set Min and Max.
type Exponential struct {
	Min time.Duration
	Max time.Duration

	attempts uint
}

// Next returns the delay to wait before the next attempt and counts the
// attempt. rnd scales the delay uniformly into [d/2, d), where d is Min
// doubled once per previous attempt and capped at Max. The jitter keeps
// devices that lost the same access point from retrying in lockstep, while
// the lower bound of d/2 keeps the growth exponential.
func (e *Exponential) Next(rnd uint32) time.Duration {
	d := e.base()
	e.attempts++
	half := d / 2
	// span*rnd/2^32: a uint32 taken modulo a span of more than 4.3 s would
	// confine the jitter to the bottom of the range.
	hi, lo := bits.Mul64(uint64(d-half), uint64(rnd))
	return half + time.Duration(hi<<32|lo>>32)
}

// Reset starts the next Next sequence at Min again, e.g. after a success.
func (e *Exponential) Reset() { e.attempts = 0 }

// Attempts reports how many delays Next has returned since the last Reset.
func (e *Exponential) Attempts() uint { return e.attempts }

func (e *Exponential) base() time.Duration {
	d := max(e.Min, 0)
	// Doubling stops at Max, so this neither overflows nor runs more than
	// log2(Max/Min) times, however many attempts were made.
	for i := uint(0); i < e.attempts && d > 0 && d < e.Max; i++ {
		if d > e.Max/2 {
			d = e.Max
		} else {
			d *= 2
		}
	}
	return min(d, max(e.Max, 0))
}
