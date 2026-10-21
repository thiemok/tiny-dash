package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"machine"
	"net/netip"
	"os"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"github.com/soypat/lneto/http/httpraw"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/tcp/rto"
	"github.com/soypat/lneto/x/xnet"
	"github.com/thiemok/tiny-dash/inky/pkg/adapters"
	"github.com/thiemok/tiny-dash/inky/pkg/inky"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
	"github.com/thiemok/tiny-dash/logkit"
	"github.com/thiemok/tiny-dash/netkit/pkg/apiurl"
	"github.com/thiemok/tiny-dash/netkit/pkg/backoff"
	"github.com/thiemok/tiny-dash/netkit/pkg/bufshare"
	"github.com/thiemok/tiny-dash/netkit/pkg/resolve"
	"github.com/thiemok/tiny-dash/netkit/pkg/wallclock"
	"github.com/thiemok/tiny-dash/picoDevice/internal/config"
	util "github.com/thiemok/tiny-dash/util/pkg"
	"github.com/thiemok/tiny-dash/util/pkg/logfmt"
)

const connTimeout = 5 * time.Second

// rxBufSize is the TCP receive window. Linux caps its MSS at half the largest
// window seen, so a 2030 B window was filled exactly by two 1015 B segments
// and advertised as zero about 95 times per image. lneto v0.3.2 took Linux's
// zero-window probe for a keepalive and did not answer it, so whenever the
// window update that followed got lost the transfer hung (THI-41); lneto
// #248/#291 answer the probe now. At 4096 B the MSS is 1460 and two full
// segments leave 1176 B open: the window only closes if Read falls behind,
// and a lost update to a non-zero window is recovered by Linux's persist
// timer sending real data, which lneto ACKs.
const rxBufSize = 4096
const pollTime = 5 * time.Millisecond

// maxFetchFailures consecutive network failures trigger a reconnect even
// while the chip reports the link up, e.g. when the DHCP lease or the route
// to the server is gone.
const maxFetchFailures = 3

// A body read that makes no progress for bodyStallTimeout, or a response
// header that does not arrive within headerTimeout, is abandoned and the
// fetch retried up to maxFetchRetries times; a healthy transfer never pauses
// for more than a few hundred milliseconds.
const (
	headerTimeout    = 10 * time.Second
	bodyStallTimeout = 5 * time.Second
	maxFetchRetries  = 2
)

// A resolved API host is trusted for dnsMaxAge, whatever its record's TTL,
// until maxDialFailures consecutive dials fail, or until the next WiFi
// reconnect. maxDialFailures stays below maxFetchFailures so a moved server
// is looked up before the link is reset.
const (
	dnsMaxAge       = time.Hour
	maxDialFailures = 2
	dnsTimeout      = 2 * time.Second
	dnsRetries      = 3
)

// logBuf bounds log memory; records that arrive while it is full are dropped
// rather than blocking the caller. At Info level the fill peaks at ~350 B
// (the startup burst), so 1 KiB leaves about 3x headroom.
var logBuf [1024]byte

const logFlushTimeout = 2 * time.Second

// The image response header is ~140 bytes (Content-Length, Content-Type,
// Date, Connection); leave room for a reverse proxy's additions.
const (
	headerBufSize  = 512
	maxHeaderPairs = 12
)

var (
	errHeaderTimeout   = errors.New("response header timeout")
	errRedirect        = errors.New("unexpected redirect")
	errHTTPStatus      = errors.New("unexpected HTTP status")
	errIncompleteImage = errors.New("incomplete image data")
	errBodyStall       = errors.New("image body stalled")
)

func main() {
	logDrainer := initLogDrainer()
	clock, floorFromBuild := newClock()
	logger, logHandler := initLogger(logDrainer.Src, clock)
	defer flushLogOnPanic(logger, logDrainer)
	time.Sleep(5 * time.Second)
	logger.Info("tiny-dash picoDevice Starting...")
	crcFreed, err := releaseCRC32Table()
	util.RequireNoError(err, "releasing the crc32 table")
	if debugBuild {
		logger.LogAttrs(context.Background(), slog.LevelInfo, "✓ crc32 slicing table released", slog.Uint64("freedBytes", crcFreed))
	} else {
		logger.LogAttrs(context.Background(), slog.LevelInfo, "✓ crc32 slicing table released")
	}
	logger.LogAttrs(context.Background(), slog.LevelInfo, "wall clock floor", slog.Time("floor", clock.Floor()), slog.Bool("fromBuild", floorFromBuild))

	apiURL := parseAPIURL(logger)
	dnsFallback := parseDNSServer(logger)
	display, requestURI := initDisplay(logger, apiURL)
	logger.Info("Initializing WiFi...")
	// Warn keeps the driver from allocating on every reconnect: its Info-level
	// rxEvent records carry eight attrs, beyond slog.Record's inline five.
	// Raise it to Info to see the event status and reason of a failed join.
	link := newWifiLink(logger, slog.New(logHandler.WithLevel(slog.LevelWarn)), dnsFallback)
	link.connect()

	lstack := &link.stack
	req := buildHTTPRequest(requestURI, apiURL)
	rxShare := bufshare.New(make([]byte, rxBufSize))
	udpc, err := newUDPConn(rxShare)
	util.RequireNoError(err, "configuring the UDP conn")
	f := &fetcher{
		lstack:   lstack,
		rstack:   lstack.StackRetrying(pollBackoff),
		resolver: newResolver(logger, apiURL, link, udpc),
		port:     apiURL.Port,
		req:      req,
		hdrBuf:   make([]byte, 0, headerBufSize),
		rxShare:  rxShare,
		// The request is the only data ever sent.
		txBuf:        make([]byte, len(req)),
		framebuf:     display.Buffer(),
		logger:       logger,
		retryBackoff: backoff.Exponential{Min: 500 * time.Millisecond, Max: 2 * time.Second},
	}
	f.rxUser = bufshare.User{Name: "fetch conn", Idle: f.connIdle}
	util.RequireNoError(f.rto.Configure(nanotime), "configuring the retransmission timer")
	ntpSync, err := newTimeSync(logger, link, udpc, clock)
	if err != nil {
		logger.Error("invalid NTP_SERVER", "server", config.NTPServer, "error", err)
	}
	util.RequireNoError(err, "invalid NTP_SERVER "+config.NTPServer)
	f.hdr.ConfigBufferGrowth(false)
	f.hdr.Reset(f.hdrBuf, maxHeaderPairs)
	refreshDuration := time.Duration(config.RefreshInterval) * time.Second
	ctx := context.Background()
	var prev, cur runtime.MemStats

	if debugBuild {
		runtime.ReadMemStats(&prev)
		logger.Info("Starting dashboard image fetch loop...", "uri", requestURI, "host", apiURL.HostHeader(), "dns", !f.resolver.IsStatic(), "expectedBytes", len(f.framebuf), "heapSys", prev.HeapSys)

		// Re-baseline so cycle 1 does not count the startup log's allocations.
		runtime.ReadMemStats(&prev)
	} else {
		logger.Info("Starting dashboard image fetch loop...", "uri", requestURI, "host", apiURL.HostHeader(), "dns", !f.resolver.IsStatic(), "expectedBytes", len(f.framebuf))
	}
	fetchFailures := 0
	for cycle := 1; ; cycle++ {
		if !link.up() || fetchFailures >= maxFetchFailures {
			// The display keeps showing the last image while offline.
			logger.LogAttrs(ctx, slog.LevelWarn, "wifi link lost; reconnecting",
				slog.Bool("linkUp", link.up()), slog.Int("fetchFailures", fetchFailures))
			link.connect()
			// The new lease may come with another DNS server or another view
			// of the network, e.g. after moving to a different access point.
			f.resolver.Invalidate()
			ntpSync.invalidate()
			fetchFailures = 0
			if debugBuild {
				// connect logs its own allocations; keep them out of the cycle's.
				runtime.ReadMemStats(&prev)
			}
		}
		// Plain-HTTP fetches go ahead whether or not the clock is synced.
		if ntpSync.syncIfDue() {
			if debugBuild {
				// The sync logs its own allocations; keep them out of the cycle's.
				runtime.ReadMemStats(&prev)
			}
		}

		stage, err := f.fetchRetrying()
		switch {
		case err == nil:
			fetchFailures = 0
		case err != errHTTPStatus && err != errRedirect:
			fetchFailures++
		}
		ok := err == nil
		if !ok {
			logger.LogAttrs(ctx, slog.LevelError, "image fetch failed", slog.String("stage", stage), slog.Any("error", err), slog.Int("failures", fetchFailures))
		} else if err = display.Update(); err != nil {
			ok = false
			logger.LogAttrs(ctx, slog.LevelError, "display update failed", slog.Any("error", err))
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "display updated successfully", slog.Int("bytesReceived", len(f.framebuf)))
		}

		if memTestGC {
			// Leaves only live objects in heapAlloc, so a leak shows as growth.
			runtime.GC()
		}
		if debugBuild {
			runtime.ReadMemStats(&cur)
			// At most five attrs: slog.Record heap-allocates beyond its inline array.
			logger.LogAttrs(ctx, slog.LevelInfo, "cycle",
				slog.Int("n", cycle),
				slog.Bool("ok", ok),
				slog.Uint64("mallocs", cur.Mallocs-prev.Mallocs),
				slog.Uint64("bytes", cur.TotalAlloc-prev.TotalAlloc),
				slog.Uint64("heapAlloc", cur.HeapAlloc),
			)
			prev = cur
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "cycle", slog.Int("n", cycle), slog.Bool("ok", ok))
		}

		if debugBuild && cycle == wifiTestOutageAfterCycle {
			link.simulateOutage(wifiTestOutageFailJoins)
		}
		if fetchFailures < maxFetchFailures {
			time.Sleep(refreshDuration)
		}
	}
}

// initLogDrainer returns a ring buffer drainer to serial running on its own
// goroutine: TinyGo's USB-CDC Write spins until the host reads, so writing to
// machine.Serial directly can stall any goroutine that logs, including the
// network and display paths. Dropped bytes are reported by the drainer itself.
func initLogDrainer() *logkit.Drainer {
	drainer := &logkit.Drainer{
		Src:      logkit.NewRingWriter(logBuf[:]),
		Dst:      machine.Serial,
		Ready:    machine.Serial.DTR,
		Writable: (&usbWaker{}).Writable,
	}
	go drainer.Run()
	return drainer
}

// flushLogOnPanic routes a panic in main through the log ring before letting
// it continue. The runtime prints the panic straight to machine.Serial and
// then halts the scheduler, so records still in the ring would be lost and,
// with the bus suspended, nothing would wake the host to deliver the message.
func flushLogOnPanic(logger *slog.Logger, drainer *logkit.Drainer) {
	if r := recover(); r != nil {
		logger.Error("panic", "error", r)
		drainer.Flush(logFlushTimeout)
		panic(r)
	}
}

// initLogger writes to ring; logfmt formats each record into one Write, so
// the ring only ever holds or drops whole records. Records carry the wall
// time once clock is synced and the time since boot (as 1970-01-01) before.
func initLogger(ring *logkit.RingWriter, clock *wallclock.Clock) (*slog.Logger, *logfmt.Handler) {
	h := logfmt.NewHandler(ring, &logfmt.Options{
		Level: slog.LevelInfo,
		Time: func() time.Time {
			now, _ := clock.Now()
			return now
		},
	})
	return slog.New(h), h
}

func initDisplay(logger *slog.Logger, apiURL apiurl.URL) (common.Display, string) {
	logger.Info("Initializing hardware...")
	hardware, err := adapters.NewPico2PicoToPiHardware()
	util.RequireNoError(err, "Failed to initialize hardware")
	logger.Info("✓ Hardware initialized")

	logger.Info("Detecting display via EEPROM...")
	display, err := inky.Auto(*hardware)
	util.RequireNoError(err, "Failed to detect display")
	logger.Info("✓ Display detected")

	width := display.Width()
	height := display.Height()
	colorDepth := display.ColorDepth()
	supportedColors := display.SupportedColors()

	logger.Info("Display Information", "resolution", fmt.Sprintf("%d x %d", width, height), "colorDepth", colorDepth, "colors", len(supportedColors))

	uri := apiURL.AppendRequestTarget(make([]byte, 0, 128), "/api/dashboard/image")
	uri = append(uri, "?width="...)
	uri = strconv.AppendInt(uri, int64(width), 10)
	uri = append(uri, "&height="...)
	uri = strconv.AppendInt(uri, int64(height), 10)
	uri = append(uri, "&colorDepth="...)
	uri = strconv.AppendInt(uri, int64(colorDepth), 10)
	uri = append(uri, "&colors="...)
	for i, c := range supportedColors {
		if i > 0 {
			uri = append(uri, ',')
		}
		uri = strconv.AppendInt(uri, int64(c), 10)
	}

	return display, string(uri)
}

func pollBackoff(uint) time.Duration { return pollTime }

// parseAPIURL fails before WiFi bring-up so a bad API_URL is reported
// immediately instead of after a DHCP round trip.
func parseAPIURL(logger *slog.Logger) apiurl.URL {
	apiURL, err := apiurl.Parse(config.APIURL)
	if err == nil && apiURL.Scheme == apiurl.SchemeHTTPS {
		// TODO(THI-32): TLS client.
		err = errors.New("https is not supported yet; use an http:// API_URL")
	}
	if err != nil {
		logger.Error("invalid API_URL", "url", config.APIURL, "error", err)
	}
	util.RequireNoError(err, "invalid API_URL "+config.APIURL)
	logger.Info("✓ API URL parsed", "url", apiURL.String(), "host", apiURL.HostHeader(), "pathPrefix", apiURL.PathPrefix, "dns", !apiURL.IsIPLiteral())
	return apiURL
}

func parseDNSServer(logger *slog.Logger) netip.Addr {
	if config.DNSServer == "" {
		return netip.Addr{}
	}
	addr, err := netip.ParseAddr(config.DNSServer)
	if err == nil && !addr.Is4() {
		err = errors.New("lneto queries DNS over IPv4 only")
	}
	if err != nil {
		logger.Error("invalid DNS_SERVER", "server", config.DNSServer, "error", err)
	}
	util.RequireNoError(err, "invalid DNS_SERVER "+config.DNSServer)
	return addr
}

// newResolver skips DNS for IP literals so local dev sessions work without a
// DNS server.
//
// DNS lookups run on udpc, which borrows the fetch conn's receive buffer,
// saving ~1.1 KB: fetch resolves the host before it opens that conn, and
// aborts the conn on every return, so after Abort lneto no longer writes into
// the buffer (see fetcher.connIdle). Each conn acquires the buffer before
// configuring it, and that fails rather than overlap while the other conn is
// still open.
func newResolver(logger *slog.Logger, apiURL apiurl.URL, link *wifiLink, udpc *udpConn) *resolve.Resolver {
	if apiURL.IsIPLiteral() {
		return resolve.Static(apiURL.Addr)
	}
	d, err := newDNSLookup(apiURL.Host, link, udpc)
	if err != nil {
		logger.Error("invalid API_URL host", "host", apiURL.Host, "error", err)
	}
	util.RequireNoError(err, "invalid API_URL host "+apiURL.Host)
	return resolve.New(apiURL.Host, d.lookup, resolve.Policy{MaxTTL: dnsMaxAge, MaxDialFailures: maxDialFailures})
}

func buildHTTPRequest(requestURI string, apiURL apiurl.URL) []byte {
	var reqHdr httpraw.HeaderV1
	reqHdr.SetMethod("GET")
	reqHdr.SetRequestTarget(requestURI)
	reqHdr.SetProtocol("HTTP/1.1")
	reqHdr.Set("Host", apiURL.HostHeader())
	reqHdr.Set("Connection", "close")
	reqBytes, err := reqHdr.AppendRequest(nil)
	util.RequireNoError(err, "building HTTP request")
	return reqBytes
}

// fetcher holds everything a refresh cycle needs so that fetch does not
// allocate.
type fetcher struct {
	lstack   *xnet.StackAsync
	rstack   xnet.StackRetrying
	resolver *resolve.Resolver
	port     uint16
	req      []byte
	conn     tcp.Conn
	hdr      httpraw.HeaderV1
	hdrBuf   []byte
	// rxShare is the conn's receive buffer, shared with DNS lookups.
	rxShare  *bufshare.Buffer
	rxUser   bufshare.User
	txBuf    []byte
	framebuf []byte
	logger   *slog.Logger
	// rto retransmits the request when its segment is lost. Otherwise lneto
	// only resends after duplicate ACKs, which a lone request never draws, so
	// the server would never answer. The conn resets it on every open.
	rto rto.Timer

	retryBackoff backoff.Exponential

	ipbuf [len("255.255.255.255")]byte
}

// fetchRetrying retries a fetch whose response header never arrived or whose
// body stalled or ended early. framebuf is only shown after a complete fetch,
// so the display keeps the last image while retries overwrite the buffer.
func (f *fetcher) fetchRetrying() (stage string, err error) {
	f.retryBackoff.Reset()
	for retry := 0; ; retry++ {
		stage, err = f.fetch()
		if retry == maxFetchRetries || !retryable(err) {
			return stage, err
		}
		wait := f.retryBackoff.Next(rand32())
		f.logger.LogAttrs(context.Background(), slog.LevelWarn, "retrying image fetch",
			slog.Any("error", err), slog.Int("retry", retry+1), slog.Duration("retryIn", wait))
		time.Sleep(wait)
	}
}

func retryable(err error) bool {
	return err == errHeaderTimeout || err == errBodyStall || err == errIncompleteImage
}

var bootTime = time.Now()

// nanotime is the monotonic clock lneto's loss recovery runs on; unlike
// UnixNano it cannot jump when the wall clock is set.
func nanotime() int64 { return int64(time.Since(bootTime)) }

// fetch streams the image into framebuf. On failure it returns the failing
// stage alongside the error so that callers need not wrap it.
func (f *fetcher) fetch() (stage string, err error) {
	addr, err := f.resolveServer()
	if err != nil {
		return "dns lookup", err
	}

	if err = f.rxShare.Acquire(&f.rxUser); err != nil {
		return "conn configure", err
	}
	conn := &f.conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             f.rxShare.Bytes(),
		TxBuf:             f.txBuf,
		TxPacketQueueSize: 3,
		RWBackoff:         pollBackoff,
		Policy:            &f.rto,
	})
	if err != nil {
		// Configure only fails on a conn that is not closed; abort it so the
		// next cycle can reconfigure instead of failing forever.
		conn.Abort()
		return "conn configure", err
	}

	lport := uint16(f.lstack.Prand32()>>17) + 1024
	// The only steady-state allocations (2 × 8 B) happen here, inside lneto:
	// under TinyGo, registering the conn stores s.Demux/s.Encapsulate as
	// bound method values (internet.makecbnode). An on-subnet server adds the
	// escaping MAC of subnetTable.hwDynamicResolve. Both need upstream fixes.
	err = f.rstack.DoDialTCP(context.Background(), conn, lport, netip.AddrPortFrom(addr, f.port), connTimeout, 3)
	if err != nil {
		f.resolver.DialFailed()
		closeConn(conn)
		return "tcp dial", err
	}
	f.resolver.DialSucceeded()

	_, err = conn.Write(f.req)
	if err != nil {
		closeConn(conn)
		return "write request", err
	}

	hdr := &f.hdr
	hdr.Reset(f.hdrBuf[:0], maxHeaderPairs)
	headerDeadline := time.Now().Add(headerTimeout)
	conn.SetReadDeadline(headerDeadline)
	headerParsed := false

	for time.Now().Before(headerDeadline) {
		n, readErr := hdr.ReadFromLimited(conn, hdr.BufferFree())
		if readErr == httpraw.ErrBufferExhausted {
			closeConn(conn)
			return "header read", readErr
		}
		if n > 0 {
			needMore, parseErr := hdr.TryParse(true)
			if parseErr != nil && !needMore {
				closeConn(conn)
				return "header parse", parseErr
			}
			if !needMore {
				headerParsed = true
				break
			}
		} else {
			time.Sleep(pollTime)
		}
	}

	if !headerParsed {
		closeConn(conn)
		return "header read", errHeaderTimeout
	}

	statusCode, statusText := hdr.Status()
	if len(statusCode) == 3 && statusCode[0] == '3' {
		location := hdr.GetFold("Location")
		closeConn(conn)
		hint := "check API_URL"
		if hasPrefixFold(location, "https://") {
			hint = "server redirected to https; set API_URL scheme to https"
		}
		f.logger.LogAttrs(context.Background(), slog.LevelError, "unexpected redirect",
			slog.String("status", bytesString(statusCode)), slog.String("statusText", bytesString(statusText)),
			slog.String("location", bytesString(location)), slog.String("hint", hint))
		return "status", errRedirect
	}
	if bytesString(statusCode) != "200" {
		closeConn(conn)
		f.logger.LogAttrs(context.Background(), slog.LevelError, "unexpected HTTP status",
			slog.String("status", bytesString(statusCode)), slog.String("statusText", bytesString(statusText)))
		return "status", errHTTPStatus
	}

	initialBody, err := hdr.Body()
	if err != nil {
		closeConn(conn)
		return "get body", err
	}
	bodyReceived := copy(f.framebuf, initialBody)

	bodyStart := time.Now()
	bodyDeadline := bodyStart.Add(30 * time.Second)
	var readErr error
	for bodyReceived < len(f.framebuf) && readErr == nil {
		// Read blocks until data arrives, so its deadline doubles as the stall
		// timer: every chunk received pushes it back.
		conn.SetReadDeadline(minTime(time.Now().Add(bodyStallTimeout), bodyDeadline))
		var n int
		n, readErr = conn.Read(f.framebuf[bodyReceived:])
		bodyReceived += n
	}

	closeConn(conn)

	if bodyReceived != len(f.framebuf) {
		err = errIncompleteImage
		if errors.Is(readErr, os.ErrDeadlineExceeded) && time.Now().Before(bodyDeadline) {
			err = errBodyStall
		}
		f.logger.LogAttrs(context.Background(), slog.LevelError, err.Error(),
			slog.Int("received", bodyReceived), slog.Int("expected", len(f.framebuf)),
			slog.Duration("after", time.Since(bodyStart)), slog.Any("readError", readErr))
		return "body read", err
	}

	return "", nil
}

// connIdle reports whether the conn is aborted: Abort zeroes the local port,
// so the conn rejects every segment, and bumps the conn ID, so the stack
// drops its port registration. fetch aborts the conn on every return.
func (f *fetcher) connIdle() bool { return f.conn.LocalPort() == 0 }

// resolveServer returns the API server's address, logging every DNS lookup,
// with its allocations in debug builds. A failed lookup falls back to the
// address from the previous one, if any.
func (f *fetcher) resolveServer() (netip.Addr, error) {
	var before, after runtime.MemStats
	if debugBuild {
		runtime.ReadMemStats(&before)
	}
	start := time.Now()
	addr, looked, err := f.resolver.Resolve(start)
	if !looked {
		return addr, err
	}
	ctx := context.Background()
	if err != nil {
		f.logger.LogAttrs(ctx, slog.LevelError, "dns lookup failed",
			slog.String("host", f.resolver.Host()), slog.Any("error", err),
			slog.Bool("usingPrevious", addr.IsValid()))
		if addr.IsValid() {
			return addr, nil
		}
		return addr, err
	}
	if debugBuild {
		runtime.ReadMemStats(&after)
		f.logger.LogAttrs(ctx, slog.LevelInfo, "✓ API host resolved",
			slog.String("host", f.resolver.Host()),
			slog.String("ip", bytesString(addr.AppendTo(f.ipbuf[:0]))),
			slog.Duration("took", time.Since(start)),
			slog.Uint64("mallocs", after.Mallocs-before.Mallocs),
			slog.Uint64("bytes", after.TotalAlloc-before.TotalAlloc),
		)
	} else {
		f.logger.LogAttrs(ctx, slog.LevelInfo, "✓ API host resolved",
			slog.String("host", f.resolver.Host()),
			slog.String("ip", bytesString(addr.AppendTo(f.ipbuf[:0]))),
			slog.Duration("took", time.Since(start)),
		)
	}
	return addr, nil
}

// closeConn shuts the read side down before closing so that a server still
// sending, as after an abandoned body, gets an RST instead of filling a window
// nobody drains; lneto has no other way to send one.
func closeConn(conn *tcp.Conn) {
	conn.CloseRead()
	conn.Close()
	for i := 0; i < 50 && !conn.State().IsClosed(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	conn.Abort()
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func hasPrefixFold(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c := b[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}

// bytesString views b as a string without copying, for log attributes that
// the logfmt handler formats before returning.
func bytesString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
