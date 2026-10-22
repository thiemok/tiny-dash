package main

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"runtime"
	"strconv"
	"time"

	"github.com/soypat/lneto/ntp"
	"github.com/soypat/lneto/udp"
	"github.com/thiemok/tiny-dash/netkit/pkg/backoff"
	"github.com/thiemok/tiny-dash/netkit/pkg/ntpquery"
	"github.com/thiemok/tiny-dash/netkit/pkg/timesync"
	"github.com/thiemok/tiny-dash/netkit/pkg/wallclock"
	"github.com/thiemok/tiny-dash/picoDevice/internal/config"
)

// The clock is synced before the first fetch, then every ntpSyncInterval and
// after every WiFi reconnect. A failed sync is retried once its backoff has
// passed, at the start of a refresh cycle; plain-HTTP fetches never wait for
// it. Each sync sends up to ntpTries requests, each with a fresh nonce.
const (
	ntpSyncInterval = 12 * time.Hour
	ntpTimeout      = 2 * time.Second
	ntpTries        = 3
	// The oscillator drifts well under a second per interval, so a larger
	// step on a re-sync means the server or the clock went wrong.
	ntpMaxStep = 10 * time.Second
	// ntpRxBufSize leaves room for a response with extension fields.
	ntpRxBufSize = 128
)

var ntpRetry = backoff.Exponential{Min: 30 * time.Second, Max: 15 * time.Minute}

// buildUnix is the build time in Unix seconds, set by tools/build.sh and the
// upload target with -ldflags "-X main.buildUnix=...". The wall clock refuses
// any time before it.
var buildUnix string

// fallbackFloorUnix is the clock's floor in builds without buildUnix,
// 2026-10-20T00:00:00Z. A constant, unlike a time.Date variable, costs no RAM
// and no init code.
const fallbackFloorUnix = 1792454400

func newClock() (clock *wallclock.Clock, fromBuild bool) {
	floor := int64(fallbackFloorUnix)
	sec, err := strconv.ParseInt(buildUnix, 10, 64)
	if fromBuild = err == nil && sec > floor; fromBuild {
		floor = sec
	}
	return wallclock.New(nanotime, time.Unix(floor, 0)), fromBuild
}

// timeSync keeps clock synced with config.NTPServer. It takes turns with the
// DNS lookups on the shared udpConn and its borrowed buffer, so a sync costs
// no RAM between syncs; it runs between fetches, on the refresh loop.
type timeSync struct {
	link   *wifiLink
	clock  *wallclock.Clock
	sched  *timesync.Schedule
	logger *slog.Logger

	host   string
	static netip.Addr
	// dns resolves host before every sync, since pool.ntp.org rotates its
	// servers; it is nil for an IP literal. Caching the address would save
	// one DialUDP (3 mallocs / 20 B) per sync, but syncs are 12 h apart, far
	// beyond the pool's TTL, and a retry after a failure should get to try
	// another server.
	dns *dnsLookup

	udp *udpConn
	cfg udp.ConnConfig
	buf []byte

	ipbuf [len("255.255.255.255")]byte
}

func newTimeSync(logger *slog.Logger, link *wifiLink, u *udpConn, clock *wallclock.Clock) (*timeSync, error) {
	interval := ntpSyncInterval
	if ntpTestSyncInterval > 0 {
		interval = time.Duration(ntpTestSyncInterval) * time.Second
	}
	t := &timeSync{
		link:   link,
		clock:  clock,
		sched:  timesync.New(timesync.Policy{Interval: interval, Retry: ntpRetry}),
		logger: logger,
		host:   config.NTPServer,
		udp:    u,
	}
	if addr, err := netip.ParseAddr(t.host); err == nil {
		if !addr.Is4() {
			return nil, errors.New("lneto talks NTP over IPv4 only")
		}
		t.static = addr
	} else {
		d, err := newDNSLookup(t.host, link, u)
		if err != nil {
			return nil, err
		}
		t.dns = d
	}

	borrowed := u.share.Bytes()
	if len(borrowed) < 2*ntpRxBufSize+ntpquery.Size {
		return nil, errors.New("borrowed NTP buffer too small")
	}
	t.cfg = udp.ConnConfig{
		RxBuf:       borrowed[:ntpRxBufSize:ntpRxBufSize],
		TxBuf:       borrowed[ntpRxBufSize : ntpRxBufSize+ntpquery.Size : ntpRxBufSize+ntpquery.Size],
		RxQueueSize: udpQueueSize,
		TxQueueSize: udpQueueSize,
		RWBackoff:   pollBackoff,
	}
	end := 2*ntpRxBufSize + ntpquery.Size
	t.buf = borrowed[ntpRxBufSize+ntpquery.Size : end : end]
	return t, nil
}

// invalidate makes the next syncIfDue sync, e.g. after a WiFi reconnect.
func (t *timeSync) invalidate() { t.sched.Invalidate() }

// syncIfDue syncs the clock when the schedule says so, logs the outcome and
// reports whether it tried. A failure leaves the clock as it was: unsynced,
// or running on its last offset.
func (t *timeSync) syncIfDue() bool {
	now := time.Now()
	if !t.sched.Due(now) {
		return false
	}
	var before, after runtime.MemStats
	if debugBuild {
		runtime.ReadMemStats(&before)
	}
	wasSynced := t.clock.Synced()
	ctx := context.Background()
	res, step, stage, err := t.sync()
	if debugBuild {
		runtime.ReadMemStats(&after)
	}
	if err != nil {
		wait := t.sched.Failed(now, rand32())
		// Whether the clock still runs on an earlier offset shows in this
		// line's time, which stays at 1970 until the first success.
		if debugBuild {
			t.logger.LogAttrs(ctx, slog.LevelWarn, "ntp sync failed",
				slog.String("stage", stage), slog.Any("error", err),
				slog.Uint64("failures", uint64(t.sched.Failures())),
				slog.Duration("retryIn", wait),
				slog.Uint64("mallocs", after.Mallocs-before.Mallocs))
		} else {
			t.logger.LogAttrs(ctx, slog.LevelWarn, "ntp sync failed",
				slog.String("stage", stage), slog.Any("error", err),
				slog.Uint64("failures", uint64(t.sched.Failures())),
				slog.Duration("retryIn", wait))
		}
		return true
	}
	t.sched.Succeeded(now)
	// offset is how far the clock moved; before the first sync the clock
	// counts from the Unix epoch at boot. This line's time is already synced.
	if debugBuild {
		t.logger.LogAttrs(ctx, slog.LevelInfo, "✓ clock synced",
			slog.Int("stratum", int(res.Stratum)),
			slog.Duration("rtt", res.RTT),
			slog.Duration("offset", step),
			slog.Uint64("mallocs", after.Mallocs-before.Mallocs),
			slog.Uint64("bytes", after.TotalAlloc-before.TotalAlloc),
		)
	} else {
		t.logger.LogAttrs(ctx, slog.LevelInfo, "✓ clock synced",
			slog.Int("stratum", int(res.Stratum)),
			slog.Duration("rtt", res.RTT),
			slog.Duration("offset", step),
		)
	}
	if wasSynced && (step > ntpMaxStep || step < -ntpMaxStep) {
		t.logger.LogAttrs(ctx, slog.LevelWarn, "large clock step on re-sync",
			slog.Duration("step", step), slog.Duration("maxStep", ntpMaxStep))
	}
	return true
}

func (t *timeSync) sync() (res ntpquery.Result, step time.Duration, stage string, err error) {
	server := t.static
	if t.dns != nil {
		if server, _, err = t.dns.lookup(t.host); err != nil {
			return res, 0, "dns lookup", err
		}
	}
	t.logger.LogAttrs(context.Background(), slog.LevelInfo, "ntp server",
		slog.String("host", t.host),
		slog.String("ip", bytesString(server.AppendTo(t.ipbuf[:0]))),
		slog.Bool("dns", t.dns != nil))

	// The lookup's registration cannot be re-targeted at the NTP server:
	// lneto v0.3.2 has no way to change an open conn's remote port, and a
	// fresh Open or Configure invalidates the registration. So a sync with a
	// lookup costs two DialUDPs, 6 mallocs / 40 B.
	if err = t.udp.acquire(t.cfg); err != nil {
		return res, 0, "conn configure", err
	}
	if err = t.udp.dial(&t.link.stack, netip.AddrPortFrom(server, ntp.ServerPort)); err != nil {
		return res, 0, "dial", err
	}
	var recv int64
	for range ntpTries {
		if res, recv, err = t.exchange(); err == nil || err == ntpquery.ErrKissOfDeath {
			break
		}
	}
	t.udp.conn.Abort()
	if err != nil {
		return res, 0, "exchange", err
	}
	step, err = t.clock.Set(res.Wall, recv)
	if err != nil {
		return res, 0, "set clock", err
	}
	return res, step, "", nil
}

// exchange sends one request and waits ntpTimeout for its answer, skipping
// datagrams that answer something else. recv is the monotonic time the answer
// was read; the read polls every pollTime, which delays it by at most that
// and skews the clock by half of it.
func (t *timeSync) exchange() (res ntpquery.Result, recv int64, err error) {
	// The nonce comes from the hardware RNG, so an off-path attacker cannot
	// guess it to forge an answer.
	nonce := uint64(rand32())<<32 | uint64(rand32())
	req := ntpquery.AppendRequest(t.buf[:0], nonce)
	sent := nanotime()
	if _, err = t.udp.conn.Write(req); err != nil {
		return res, 0, err
	}
	t.udp.conn.SetReadDeadline(time.Now().Add(ntpTimeout))
	for {
		n, err := t.udp.conn.Read(t.buf[:cap(t.buf)])
		recv = nanotime()
		if err != nil {
			return res, recv, err
		}
		res, err = ntpquery.ParseResponse(t.buf[:n], nonce, sent, recv)
		if err != ntpquery.ErrNotResponse {
			return res, recv, err
		}
	}
}
