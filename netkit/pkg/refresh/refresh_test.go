package refresh

import (
	"testing"
	"time"

	"github.com/thiemok/tiny-dash/netkit/pkg/backoff"
)

// rndTop makes backoff.Exponential.Next return just under its full delay.
const rndTop = ^uint32(0)

var (
	t0     = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	policy = Policy{
		Default:  time.Minute,
		Min:      2 * time.Minute,
		Max:      4 * time.Hour,
		PressMin: 30 * time.Second,
		Retry:    backoff.Exponential{Min: 30 * time.Second, Max: 10 * time.Minute},
	}
)

func TestSucceeded(t *testing.T) {
	tests := []struct {
		name    string
		hint    time.Duration
		ok      bool
		wantD   time.Duration
		wantSrc Source
	}{
		{"no hint uses default", 0, false, time.Minute, SourceDefault},
		{"hint within bounds", 15 * time.Minute, true, 15 * time.Minute, SourceHint},
		{"hint below min", 10 * time.Second, true, 2 * time.Minute, SourceHint},
		{"zero hint", 0, true, 2 * time.Minute, SourceHint},
		{"hint above max", 24 * time.Hour, true, 4 * time.Hour, SourceHint},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(policy)
			d, src := s.Succeeded(t0, tt.hint, tt.ok)
			if d != tt.wantD || src != tt.wantSrc {
				t.Fatalf("Succeeded = %v, %v; want %v, %v", d, src, tt.wantD, tt.wantSrc)
			}
			if !s.Next().Equal(t0.Add(tt.wantD)) {
				t.Fatalf("Next = %v, want %v", s.Next(), t0.Add(tt.wantD))
			}
		})
	}
}

func TestFailedBacksOffAndSuccessResets(t *testing.T) {
	s := New(policy)
	var got []time.Duration
	for range 7 {
		got = append(got, s.Failed(t0, rndTop).Round(time.Second))
	}
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("retry delays = %v, want %v", got, want)
		}
	}
	if s.Failures() != 7 {
		t.Fatalf("Failures = %d, want 7", s.Failures())
	}

	// The server's hint never stretches a retry, and a success starts the
	// backoff over.
	s.Succeeded(t0, 4*time.Hour, true)
	if s.Failures() != 0 {
		t.Fatalf("Failures after success = %d, want 0", s.Failures())
	}
	if d := s.Failed(t0, rndTop).Round(time.Second); d != 30*time.Second {
		t.Fatalf("first retry after success = %v, want 30s", d)
	}
}

func TestPress(t *testing.T) {
	t.Run("before any fetch is due at once", func(t *testing.T) {
		s := New(policy)
		if due := s.Press(t0); !due.Equal(t0) {
			t.Fatalf("Press = %v, want %v", due, t0)
		}
	})
	t.Run("long after the last fetch is due at once", func(t *testing.T) {
		s := New(policy)
		s.Succeeded(t0, time.Hour, true)
		now := t0.Add(10 * time.Minute)
		if due := s.Press(now); !due.Equal(now) || !s.Next().Equal(now) {
			t.Fatalf("Press = %v, Next = %v; want %v", due, s.Next(), now)
		}
	})
	t.Run("right after the last fetch waits for press min", func(t *testing.T) {
		s := New(policy)
		s.Succeeded(t0, time.Hour, true)
		want := t0.Add(policy.PressMin)
		if due := s.Press(t0.Add(10 * time.Second)); !due.Equal(want) {
			t.Fatalf("Press = %v, want %v", due, want)
		}
	})
	t.Run("is not held back by the hint's min", func(t *testing.T) {
		s := New(policy)
		s.Succeeded(t0, time.Hour, true)
		now := t0.Add(40 * time.Second) // past PressMin, within Min
		if due := s.Press(now); !due.Equal(now) {
			t.Fatalf("Press = %v, want %v", due, now)
		}
	})
	t.Run("cuts a failure backoff short", func(t *testing.T) {
		// Only a successful fetch refreshes the panel, so a press during the
		// backoff is spaced from that one, not from the failed attempt.
		s := New(policy)
		s.Succeeded(t0, time.Hour, true)
		failedAt := t0.Add(time.Hour)
		for range 5 {
			s.Failed(failedAt, rndTop)
		}
		now := failedAt.Add(time.Second)
		if due := s.Press(now); !due.Equal(now) {
			t.Fatalf("Press = %v, want %v", due, now)
		}
	})
	t.Run("never postpones an earlier fetch", func(t *testing.T) {
		p := policy
		p.Default = 10 * time.Second // below PressMin
		s := New(p)
		s.Succeeded(t0, 0, false)
		want := t0.Add(p.Default)
		if due := s.Press(t0.Add(5 * time.Second)); !due.Equal(want) {
			t.Fatalf("Press = %v, want %v", due, want)
		}
	})
}

func TestParseHint(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"300", 300 * time.Second, true},
		{" 60 ", time.Minute, true},
		{"0", 0, true},
		{"999999999", 999999999 * time.Second, true},
		{"", 0, false},
		{"  ", 0, false},
		{"-5", 0, false},
		{"1.5", 0, false},
		{"10s", 0, false},
		{"Fri, 23 Oct 2026 12:00:00 GMT", 0, false},
		{"1234567890", 0, false},
	}
	for _, tt := range tests {
		d, ok := ParseHint([]byte(tt.in))
		if d != tt.want || ok != tt.ok {
			t.Errorf("ParseHint(%q) = %v, %v; want %v, %v", tt.in, d, ok, tt.want, tt.ok)
		}
	}
}

func TestDoesNotAllocate(t *testing.T) {
	s := New(policy)
	b := []byte("300")
	etag := []byte(`"0123456789abcdef"`)
	var dst [ETagLen]byte
	allocs := testing.AllocsPerRun(100, func() {
		ParseETag(etag, &dst)
		d, ok := ParseHint(b)
		s.Succeeded(t0, d, ok)
		s.Failed(t0, rndTop)
		s.Press(t0)
	})
	if allocs != 0 {
		t.Fatalf("allocs = %v, want 0", allocs)
	}
}

func TestParseETag(t *testing.T) {
	tests := []struct {
		in string
		ok bool
	}{
		{`"0123456789abcdef"`, true},
		{`0123456789abcdef`, false},
		{`W/"0123456789abcdef"`, false},
		{`"0123456789ABCDEF"`, false},
		{`"0123456789abcde"`, false},
		{`"` + NoETag + `"`, false},
		{``, false},
	}
	for _, tt := range tests {
		var dst [ETagLen]byte
		ok := ParseETag([]byte(tt.in), &dst)
		if ok != tt.ok {
			t.Errorf("ParseETag(%q) ok = %v, want %v", tt.in, ok, tt.ok)
		}
		if ok && string(dst[:]) != tt.in[1:len(tt.in)-1] {
			t.Errorf("ParseETag(%q) = %q", tt.in, dst[:])
		}
	}
	if len(NoETag) != ETagLen {
		t.Fatalf("len(NoETag) = %d, want %d", len(NoETag), ETagLen)
	}
}
