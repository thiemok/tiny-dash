package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"machine"
	"net/netip"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"github.com/soypat/lneto/http/httpraw"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/x/xnet"
	"github.com/thiemok/tiny-dash/inky/pkg/adapters"
	"github.com/thiemok/tiny-dash/inky/pkg/inky"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
	"github.com/thiemok/tiny-dash/logkit"
	"github.com/thiemok/tiny-dash/netkit/pkg/apiurl"
	"github.com/thiemok/tiny-dash/picoDevice/internal/config"
	util "github.com/thiemok/tiny-dash/util/pkg"
	"github.com/thiemok/tiny-dash/util/pkg/logfmt"
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
const pollTime = 5 * time.Millisecond

// maxFetchFailures consecutive network failures trigger a reconnect even
// while the chip reports the link up, e.g. when the DHCP lease or the route
// to the server is gone.
const maxFetchFailures = 3

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
)

func main() {
	logDrainer := initLogDrainer()
	logger, logHandler := initLogger(logDrainer.Src)
	defer flushLogOnPanic(logger, logDrainer)
	time.Sleep(5 * time.Second)
	logger.Info("tiny-dash picoDevice Starting...")

	apiURL, svAddr := parseAPIURL(logger)
	display, requestURI := initDisplay(logger, apiURL)
	logger.Info("Initializing WiFi...")
	// Warn keeps the driver from allocating on every reconnect: its Info-level
	// rxEvent records carry eight attrs, beyond slog.Record's inline five.
	// Raise it to Info to see the event status and reason of a failed join.
	link := newWifiLink(logger, slog.New(logHandler.WithLevel(slog.LevelWarn)))
	link.connect()

	lstack := &link.stack
	req := buildHTTPRequest(requestURI, apiURL)
	f := &fetcher{
		lstack: lstack,
		rstack: lstack.StackRetrying(pollBackoff),
		addr:   svAddr,
		req:    req,
		hdrBuf: make([]byte, 0, headerBufSize),
		rxBuf:  make([]byte, tcpbufsize),
		// The request is the only data ever sent.
		txBuf:    make([]byte, len(req)),
		framebuf: display.Buffer(),
		logger:   logger,
	}
	f.hdr.ConfigBufferGrowth(false)
	f.hdr.Reset(f.hdrBuf, maxHeaderPairs)
	refreshDuration := time.Duration(config.RefreshInterval) * time.Second
	ctx := context.Background()
	var prev, cur runtime.MemStats

	if debugBuild {
		runtime.ReadMemStats(&prev)
		logger.Info("Starting dashboard image fetch loop...", "uri", requestURI, "host", apiURL.HostHeader(), "addr", svAddr.String(), "expectedBytes", len(f.framebuf), "heapSys", prev.HeapSys)

		// Re-baseline so cycle 1 does not count the startup log's allocations.
		runtime.ReadMemStats(&prev)
	} else {
		logger.Info("Starting dashboard image fetch loop...", "uri", requestURI, "host", apiURL.HostHeader(), "addr", svAddr.String(), "expectedBytes", len(f.framebuf))
	}
	fetchFailures := 0
	for cycle := 1; ; cycle++ {
		if !link.up() || fetchFailures >= maxFetchFailures {
			// The display keeps showing the last image while offline.
			logger.LogAttrs(ctx, slog.LevelWarn, "wifi link lost; reconnecting",
				slog.Bool("linkUp", link.up()), slog.Int("fetchFailures", fetchFailures))
			link.connect()
			fetchFailures = 0
			if debugBuild {
				// connect logs its own allocations; keep them out of the cycle's.
				runtime.ReadMemStats(&prev)
			}
		}

		stage, err := f.fetch()
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
// the ring only ever holds or drops whole records.
func initLogger(ring *logkit.RingWriter) (*slog.Logger, *logfmt.Handler) {
	h := logfmt.NewHandler(ring, &logfmt.Options{Level: slog.LevelInfo})
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
func parseAPIURL(logger *slog.Logger) (apiurl.URL, netip.AddrPort) {
	apiURL, err := apiurl.Parse(config.APIURL)
	if err == nil && apiURL.Scheme == apiurl.SchemeHTTPS {
		// TODO(THI-32): TLS client.
		err = errors.New("https is not supported yet; use an http:// API_URL")
	}
	svAddr, ok := apiURL.AddrPort()
	if err == nil && !ok {
		// TODO(THI-29): resolve host names via DNS.
		err = errors.New("host name " + apiURL.Host + " needs DNS, which is not supported yet; use an IP literal")
	}
	if err != nil {
		logger.Error("invalid API_URL", "url", config.APIURL, "error", err)
	}
	util.RequireNoError(err, "invalid API_URL "+config.APIURL)
	logger.Info("✓ API URL parsed", "url", apiURL.String(), "host", apiURL.HostHeader(), "pathPrefix", apiURL.PathPrefix)
	return apiURL, svAddr
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
	addr     netip.AddrPort
	req      []byte
	conn     tcp.Conn
	hdr      httpraw.HeaderV1
	hdrBuf   []byte
	rxBuf    []byte
	txBuf    []byte
	framebuf []byte
	logger   *slog.Logger
}

// fetch streams the image into framebuf. On failure it returns the failing
// stage alongside the error so that callers need not wrap it.
func (f *fetcher) fetch() (stage string, err error) {
	conn := &f.conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             f.rxBuf,
		TxBuf:             f.txBuf,
		TxPacketQueueSize: 3,
		RWBackoff:         pollBackoff,
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
	err = f.rstack.DoDialTCP(conn, lport, f.addr, connTimeout, 3)
	if err != nil {
		closeConn(conn)
		return "tcp dial", err
	}

	_, err = conn.Write(f.req)
	if err != nil {
		closeConn(conn)
		return "write request", err
	}

	hdr := &f.hdr
	hdr.Reset(f.hdrBuf[:0], maxHeaderPairs)
	headerDeadline := time.Now().Add(10 * time.Second)
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

	bodyDeadline := time.Now().Add(30 * time.Second)
	conn.SetReadDeadline(bodyDeadline)
	for bodyReceived < len(f.framebuf) && time.Now().Before(bodyDeadline) {
		n, _ := conn.Read(f.framebuf[bodyReceived:])
		if n > 0 {
			bodyReceived += n
		} else {
			time.Sleep(pollTime)
		}
	}

	closeConn(conn)

	if bodyReceived != len(f.framebuf) {
		f.logger.LogAttrs(context.Background(), slog.LevelError, "incomplete image data",
			slog.Int("received", bodyReceived), slog.Int("expected", len(f.framebuf)))
		return "body read", errIncompleteImage
	}

	return "", nil
}

func closeConn(conn *tcp.Conn) {
	conn.Close()
	for i := 0; i < 50 && !conn.State().IsClosed(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	conn.Abort()
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
