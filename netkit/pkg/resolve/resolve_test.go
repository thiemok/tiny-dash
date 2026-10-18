package resolve

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

var (
	addrA   = netip.MustParseAddr("192.0.2.20")
	addrB   = netip.MustParseAddr("192.0.2.21")
	errDown = errors.New("dns down")
)

type fakeDNS struct {
	addr    netip.Addr
	ttl     time.Duration
	err     error
	queries int
	host    string
}

func (f *fakeDNS) lookup(host string) (netip.Addr, time.Duration, error) {
	f.queries++
	f.host = host
	return f.addr, f.ttl, f.err
}

func mustResolve(t *testing.T, r *Resolver, now time.Time, wantAddr netip.Addr, wantLooked bool) {
	t.Helper()
	addr, looked, err := r.Resolve(now)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if addr != wantAddr || looked != wantLooked {
		t.Fatalf("Resolve = %v, looked %v; want %v, looked %v", addr, looked, wantAddr, wantLooked)
	}
}

func TestStaticNeverLooksUp(t *testing.T) {
	r := Static(addrA)
	r.Invalidate()
	for range 5 {
		r.DialFailed()
	}
	mustResolve(t, r, time.Unix(0, 0), addrA, false)
	if !r.IsStatic() || r.Host() != "192.0.2.20" {
		t.Errorf("IsStatic = %v, Host = %q", r.IsStatic(), r.Host())
	}
}

func TestCachesUntilTTL(t *testing.T) {
	dns := &fakeDNS{addr: addrA, ttl: time.Minute}
	r := New("api.example", dns.lookup, Policy{MaxTTL: time.Hour})
	t0 := time.Unix(1000, 0)

	mustResolve(t, r, t0, addrA, true)
	if dns.host != "api.example" {
		t.Errorf("looked up %q", dns.host)
	}
	if got := r.Expires(); !got.Equal(t0.Add(time.Minute)) {
		t.Errorf("Expires = %v, want %v", got, t0.Add(time.Minute))
	}
	mustResolve(t, r, t0.Add(59*time.Second), addrA, false)

	dns.addr = addrB
	mustResolve(t, r, t0.Add(time.Minute), addrB, true)
	if dns.queries != 2 {
		t.Errorf("queries = %d, want 2", dns.queries)
	}
}

func TestTTLBounds(t *testing.T) {
	policy := Policy{MinTTL: 5 * time.Minute, MaxTTL: time.Hour}
	tests := []struct {
		name string
		ttl  time.Duration
		want time.Duration
	}{
		{"unknown TTL cached for MaxTTL", 0, time.Hour},
		{"short TTL raised to MinTTL", time.Second, 5 * time.Minute},
		{"long TTL capped at MaxTTL", 24 * time.Hour, time.Hour},
		{"TTL within bounds kept", 10 * time.Minute, 10 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dns := &fakeDNS{addr: addrA, ttl: tt.ttl}
			r := New("h", dns.lookup, policy)
			t0 := time.Unix(1000, 0)
			mustResolve(t, r, t0, addrA, true)
			if got := r.Expires().Sub(t0); got != tt.want {
				t.Errorf("cached for %v, want %v", got, tt.want)
			}
		})
	}
}

func TestZeroPolicyCachesUntilInvalidated(t *testing.T) {
	dns := &fakeDNS{addr: addrA}
	r := New("h", dns.lookup, Policy{})
	t0 := time.Unix(1000, 0)
	mustResolve(t, r, t0, addrA, true)
	if !r.Expires().IsZero() {
		t.Errorf("Expires = %v, want zero", r.Expires())
	}
	mustResolve(t, r, t0.Add(1000*time.Hour), addrA, false)
	r.Invalidate()
	mustResolve(t, r, t0.Add(1000*time.Hour), addrA, true)
}

func TestDialFailuresTriggerLookup(t *testing.T) {
	dns := &fakeDNS{addr: addrA}
	r := New("h", dns.lookup, Policy{MaxTTL: time.Hour, MaxDialFailures: 2})
	now := time.Unix(1000, 0)
	mustResolve(t, r, now, addrA, true)

	r.DialFailed()
	r.DialSucceeded()
	r.DialFailed()
	mustResolve(t, r, now, addrA, false)

	r.DialFailed()
	dns.addr = addrB
	mustResolve(t, r, now, addrB, true)

	r.DialFailed()
	mustResolve(t, r, now, addrB, false)
}

func TestFailedLookupKeepsStaleAddr(t *testing.T) {
	dns := &fakeDNS{err: errDown}
	r := New("h", dns.lookup, Policy{MaxTTL: time.Hour})
	now := time.Unix(1000, 0)

	addr, looked, err := r.Resolve(now)
	if err != errDown || addr.IsValid() || !looked {
		t.Fatalf("Resolve = %v, %v, %v; want invalid addr, looked, errDown", addr, looked, err)
	}

	dns.addr, dns.err = addrA, nil
	mustResolve(t, r, now, addrA, true)

	r.Invalidate()
	dns.err = errDown
	addr, looked, err = r.Resolve(now)
	if err != errDown || addr != addrA || !looked {
		t.Fatalf("Resolve = %v, %v, %v; want stale %v, looked, errDown", addr, looked, err, addrA)
	}
	dns.err = nil
	mustResolve(t, r, now, addrA, true)
	if dns.queries != 4 {
		t.Errorf("queries = %d, want 4", dns.queries)
	}
}

func TestNilLookup(t *testing.T) {
	r := New("h", nil, Policy{})
	if _, _, err := r.Resolve(time.Unix(0, 0)); err != ErrNoLookup {
		t.Fatalf("err = %v, want ErrNoLookup", err)
	}
}

func TestCachedResolveDoesNotAllocate(t *testing.T) {
	dns := &fakeDNS{addr: addrA, ttl: time.Hour}
	r := New("h", dns.lookup, Policy{MaxDialFailures: 3})
	now := time.Unix(1000, 0)
	mustResolve(t, r, now, addrA, true)
	allocs := testing.AllocsPerRun(100, func() {
		r.Resolve(now)
		r.DialFailed()
		r.DialSucceeded()
	})
	if allocs != 0 {
		t.Errorf("cached Resolve allocated %v times per run", allocs)
	}
}
