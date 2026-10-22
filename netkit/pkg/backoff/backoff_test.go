package backoff

import (
	"math"
	"testing"
	"time"
)

func TestNextBounds(t *testing.T) {
	e := Exponential{Min: time.Second, Max: 60 * time.Second}
	bases := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 60}
	for i, base := range bases {
		base *= time.Second
		if got := e.Attempts(); got != uint(i) {
			t.Fatalf("Attempts = %d, want %d", got, i)
		}
		lo := e
		if got := lo.Next(0); got != base/2 {
			t.Errorf("attempt %d: Next(0) = %v, want %v", i, got, base/2)
		}
		hi := e
		if got := hi.Next(math.MaxUint32); got < base-base/1e9 || got >= base {
			t.Errorf("attempt %d: Next(max) = %v, want just below %v", i, got, base)
		}
		mid := e
		if got := mid.Next(1 << 31); got != base/2+base/4 {
			t.Errorf("attempt %d: Next(2^31) = %v, want %v", i, got, base/2+base/4)
		}
		e.Next(0)
	}
}

func TestReset(t *testing.T) {
	e := Exponential{Min: time.Second, Max: time.Minute}
	for range 10 {
		e.Next(0)
	}
	e.Reset()
	if got := e.Attempts(); got != 0 {
		t.Fatalf("Attempts after Reset = %d", got)
	}
	if got := e.Next(0); got != 500*time.Millisecond {
		t.Errorf("Next after Reset = %v, want 500ms", got)
	}
}

func TestManyAttemptsStayCapped(t *testing.T) {
	e := Exponential{Min: time.Nanosecond, Max: math.MaxInt64}
	for range 200 {
		if got := e.Next(math.MaxUint32); got < 0 {
			t.Fatalf("Next overflowed: %v", got)
		}
	}
	if got := e.base(); got != math.MaxInt64 {
		t.Errorf("base after 200 attempts = %v, want MaxInt64", got)
	}
}

func TestDegenerateConfig(t *testing.T) {
	tests := []struct {
		name     string
		min, max time.Duration
		want     time.Duration
	}{
		{"zero value", 0, 0, 0},
		{"max below min", 10 * time.Second, time.Second, time.Second},
		{"negative", -time.Second, -time.Second, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Exponential{Min: tt.min, Max: tt.max}
			for range 5 {
				e.Next(0)
			}
			if got := e.base(); got != tt.want {
				t.Errorf("base = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNextDoesNotAllocate(t *testing.T) {
	e := Exponential{Min: time.Second, Max: time.Minute}
	allocs := testing.AllocsPerRun(100, func() { e.Next(12345) })
	if allocs != 0 {
		t.Errorf("Next allocates %v times per call", allocs)
	}
}
