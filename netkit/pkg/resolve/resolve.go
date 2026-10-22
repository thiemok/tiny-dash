// Package resolve caches the address of one host name and decides when to
// look it up again: when the record's TTL runs out, after a number of
// consecutive failed dials, or when the caller invalidates it, e.g. after a
// network reconnect. It does no I/O; a LookupFunc runs the query, so any DNS
// client can back it.
//
// A cached Resolve does not allocate, so it can sit in allocation-free
// request loops.
package resolve

import (
	"errors"
	"net/netip"
	"time"
)

var ErrNoLookup = errors.New("resolve: no lookup function")

// LookupFunc resolves host to an address. ttl is the record's time to live,
// or 0 when the client does not report it.
type LookupFunc func(host string) (addr netip.Addr, ttl time.Duration, err error)

// Policy controls how long an address is trusted.
type Policy struct {
	// MinTTL and MaxTTL bound the time an address is cached; a TTL of 0
	// (unknown) caches it for MaxTTL. MinTTL keeps records with a zero or
	// tiny TTL, as some LAN resolvers hand out, from being queried on every
	// request. A zero MaxTTL caches addresses until a dial fails or the
	// cache is invalidated.
	MinTTL, MaxTTL time.Duration
	// MaxDialFailures consecutive failed dials make the address stale; 0
	// disables the check.
	MaxDialFailures int
}

// Resolver caches the address of a host name. The zero value is not usable;
// create one with New or Static.
type Resolver struct {
	host   string
	lookup LookupFunc
	policy Policy

	addr     netip.Addr
	expires  time.Time
	stale    bool
	failures int
	static   bool
}

// New returns a Resolver for host that looks it up with lookup.
func New(host string, lookup LookupFunc, policy Policy) *Resolver {
	return &Resolver{host: host, lookup: lookup, policy: policy, stale: true}
}

// Static returns a Resolver that always yields addr and never looks it up,
// for hosts configured as IP literals.
func Static(addr netip.Addr) *Resolver {
	return &Resolver{host: addr.String(), addr: addr, static: true}
}

// Host returns the host name the Resolver looks up.
func (r *Resolver) Host() string { return r.host }

// IsStatic reports whether the Resolver was created by Static.
func (r *Resolver) IsStatic() bool { return r.static }

// Resolve returns the host's address, looking it up first when there is
// none cached or the cached one is stale; looked reports whether it did.
//
// When a lookup fails but an earlier address is cached, Resolve returns that
// address along with the error, so the caller can decide to keep using it.
// The address stays stale and the next Resolve looks it up again.
func (r *Resolver) Resolve(now time.Time) (addr netip.Addr, looked bool, err error) {
	if !r.needsLookup(now) {
		return r.addr, false, nil
	}
	if r.lookup == nil {
		return r.addr, false, ErrNoLookup
	}
	addr, ttl, err := r.lookup(r.host)
	if err != nil {
		return r.addr, true, err
	}
	r.addr = addr
	r.stale = false
	r.failures = 0
	r.expires = time.Time{}
	if ttl = r.cacheFor(ttl); ttl > 0 {
		r.expires = now.Add(ttl)
	}
	return addr, true, nil
}

func (r *Resolver) needsLookup(now time.Time) bool {
	if r.static {
		return false
	}
	return r.stale || !r.expires.IsZero() && !now.Before(r.expires)
}

func (r *Resolver) cacheFor(ttl time.Duration) time.Duration {
	p := r.policy
	if ttl <= 0 || p.MaxTTL > 0 && ttl > p.MaxTTL {
		ttl = p.MaxTTL
	}
	return max(ttl, p.MinTTL)
}

// Expires returns when the cached address goes stale by age, or the zero
// time when it only does so through failed dials or Invalidate.
func (r *Resolver) Expires() time.Time { return r.expires }

// DialFailed records a failed connection to the cached address. Reaching
// Policy.MaxDialFailures makes the address stale.
func (r *Resolver) DialFailed() {
	r.failures++
	if r.policy.MaxDialFailures > 0 && r.failures >= r.policy.MaxDialFailures {
		r.Invalidate()
	}
}

// DialSucceeded resets the failed dial count.
func (r *Resolver) DialSucceeded() { r.failures = 0 }

// Invalidate makes the next Resolve look the host up again. The cached
// address is kept as a fallback should that lookup fail.
func (r *Resolver) Invalidate() {
	if !r.static {
		r.stale = true
		r.failures = 0
	}
}
