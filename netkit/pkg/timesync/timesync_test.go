package timesync

import (
	"testing"
	"time"

	"github.com/thiemok/tiny-dash/netkit/pkg/backoff"
)

var (
	t0     = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	policy = Policy{
		Interval: 12 * time.Hour,
		Retry:    backoff.Exponential{Min: 30 * time.Second, Max: 15 * time.Minute},
	}
)

// rndTop makes backoff.Exponential.Next return just under its full delay.
const rndTop = 1<<32 - 1

func TestFirstSyncIsDue(t *testing.T) {
	s := New(policy)
	if !s.Due(t0) || !s.Next().IsZero() {
		t.Fatalf("new schedule: Due = %v, Next = %v; want due at once", s.Due(t0), s.Next())
	}
}

func TestIntervalAfterSuccess(t *testing.T) {
	s := New(policy)
	s.Succeeded(t0)
	if s.Due(t0.Add(12*time.Hour - time.Second)) {
		t.Error("sync due before the interval passed")
	}
	if !s.Due(t0.Add(12 * time.Hour)) {
		t.Error("sync not due after the interval")
	}
	if want := t0.Add(12 * time.Hour); !s.Next().Equal(want) {
		t.Errorf("Next() = %v, want %v", s.Next(), want)
	}
}

func TestBackoffAfterFailures(t *testing.T) {
	s := New(policy)
	now := t0
	var waits []time.Duration
	for range 7 {
		wait := s.Failed(now, rndTop)
		if s.Due(now.Add(wait - time.Millisecond)) {
			t.Fatalf("sync due before its %v backoff passed", wait)
		}
		now = now.Add(wait)
		if !s.Due(now) {
			t.Fatalf("sync not due after its %v backoff", wait)
		}
		waits = append(waits, wait.Round(time.Second))
	}
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	for i := range want {
		if waits[i] != want[i] {
			t.Errorf("waits = %v, want %v", waits, want)
			break
		}
	}
	if s.Failures() != 7 {
		t.Errorf("Failures() = %d, want 7", s.Failures())
	}

	s.Succeeded(now)
	if s.Failures() != 0 {
		t.Errorf("Failures() after success = %d, want 0", s.Failures())
	}
	if wait := s.Failed(now, rndTop); wait.Round(time.Second) != 30*time.Second {
		t.Errorf("first wait after a success = %v, want 30s", wait)
	}
}

func TestInvalidate(t *testing.T) {
	s := New(policy)
	s.Succeeded(t0)
	s.Invalidate()
	if !s.Due(t0) || !s.Next().IsZero() {
		t.Fatal("sync not due after Invalidate")
	}
	s.Failed(t0, rndTop)
	s.Invalidate()
	if !s.Due(t0) {
		t.Fatal("sync not due after Invalidate during backoff")
	}
	if wait := s.Failed(t0, rndTop); wait.Round(time.Second) != time.Minute {
		t.Errorf("wait after Invalidate = %v, want the backoff to keep growing to 1m", wait)
	}
}

func TestScheduleDoesNotAllocate(t *testing.T) {
	s := New(policy)
	allocs := testing.AllocsPerRun(100, func() {
		s.Due(t0)
		s.Failed(t0, rndTop)
		s.Succeeded(t0)
		s.Invalidate()
		s.Next()
	})
	if allocs != 0 {
		t.Errorf("schedule allocated %v times per run", allocs)
	}
}
