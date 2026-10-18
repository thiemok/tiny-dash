package main

import (
	"context"
	"fmt"
	"log/slog"
	"machine"
	"net/netip"
	"runtime"
	"strconv"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/lneto/http/httpraw"
	"github.com/soypat/lneto/tcp"
	"github.com/soypat/lneto/x/xnet"
	"github.com/thiemok/tiny-dash/inky/pkg/adapters"
	"github.com/thiemok/tiny-dash/inky/pkg/inky"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
	"github.com/thiemok/tiny-dash/logkit"
	"github.com/thiemok/tiny-dash/picoDevice/internal/config"
	util "github.com/thiemok/tiny-dash/util/pkg"
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
const pollTime = 5 * time.Millisecond

// logBuf bounds log memory; records that arrive while it is full are dropped
// rather than blocking the caller. At Info level the fill peaks at ~350 B
// (the startup burst), so 1 KiB leaves about 3x headroom.
var logBuf [1024]byte

const logFlushTimeout = 2 * time.Second

func main() {
	logDrainer := initLogDrainer()
	logRing := logDrainer.Src
	logger := newLogger(logRing, slog.LevelInfo)
	defer flushLogOnPanic(logger, logDrainer)
	time.Sleep(5 * time.Second)
	logger.Info("tiny-dash picoDevice Starting...")

	display, requestURI := initDisplay(logger)
	stack := initWifiStack(logger, newLogger(logRing, slog.LevelWarn))

	go loopForeverStack(stack)

	// DHCP
	dhcpResults, err := stack.SetupWithDHCP(cywnet.DHCPConfig{})
	util.RequireNoError(err, "DHCP failed")
	logger.Info("✓ DHCP complete", "ip", netip.AddrFrom4(dhcpResults.AssignedAddr4).String())

	svAddr := parseServerAddr()
	reqBytes := buildHTTPRequest(requestURI, svAddr)

	// Pre-allocate buffers to avoid per-iteration allocation
	framebuf := display.Buffer()
	expectedSize := len(framebuf)
	headerBuf := make([]byte, 256)
	rxBuf := make([]byte, tcpbufsize)
	txBuf := make([]byte, tcpbufsize)

	lstack := stack.LnetoStack()
	rstack := lstack.StackRetrying(func(uint) time.Duration { return pollTime })
	refreshDuration := time.Duration(config.RefreshInterval) * time.Second

	logger.Info("Starting dashboard image fetch loop...", "uri", requestURI, "expectedBytes", expectedSize)

	ctx := context.Background()
	var memStats, prevMemStats runtime.MemStats
	if debugBuild {
		runtime.ReadMemStats(&prevMemStats)
		logger.LogAttrs(ctx, slog.LevelInfo, "heap",
			slog.Uint64("heapSys", prevMemStats.HeapSys),
			slog.Uint64("heapAlloc", prevMemStats.HeapAlloc),
		)
	}
	for cycle := 1; ; cycle++ {
		cycleStart := time.Now()
		// Fetch image on a separate goroutine
		type fetchResult struct{ err error }
		ch := make(chan fetchResult, 1)
		go func() {
			ch <- fetchResult{err: fetchImage(&rstack, lstack, svAddr, reqBytes, framebuf, headerBuf, rxBuf, txBuf, expectedSize, logger)}
		}()

		result := <-ch

		if result.err != nil {
			logger.Error("image fetch failed", "error", result.err)
		} else {
			err = display.Update()
			if err != nil {
				logger.Error("display update failed", "error", err)
			} else {
				logger.Info("display updated successfully", "bytesReceived", expectedSize)
			}
		}

		if debugBuild {
			// mallocs and allocBytes cover everything since the previous report,
			// including that report's own log line and the sleep in between.
			// LogAttrs with at most five Attrs keeps the record inline, so this
			// line adds no allocations of its own beyond the handler's.
			runtime.ReadMemStats(&memStats)
			logger.LogAttrs(ctx, slog.LevelInfo, "refresh cycle done",
				slog.Int("cycle", cycle),
				slog.Int64("cycleMs", time.Since(cycleStart).Milliseconds()),
				slog.Uint64("mallocs", memStats.Mallocs-prevMemStats.Mallocs),
				slog.Uint64("allocBytes", memStats.TotalAlloc-prevMemStats.TotalAlloc),
				slog.Uint64("heapAlloc", memStats.HeapAlloc),
			)
			prevMemStats = memStats
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "refresh cycle done",
				slog.Int("cycle", cycle),
				slog.Int64("cycleMs", time.Since(cycleStart).Milliseconds()),
			)
		}

		time.Sleep(refreshDuration)
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

// newLogger writes to ring. Every record costs heap allocations in the
// TextHandler, so the WiFi driver gets its own, quieter logger.
func newLogger(ring *logkit.RingWriter, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(ring, &slog.HandlerOptions{Level: level}))
}

func initDisplay(logger *slog.Logger) (common.Display, string) {
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

	colorsParam := ""
	for i, c := range supportedColors {
		if i > 0 {
			colorsParam += ","
		}
		colorsParam += strconv.Itoa(int(c))
	}

	requestURI := "/api/dashboard/image?width=" + strconv.Itoa(width) +
		"&height=" + strconv.Itoa(height) +
		"&colorDepth=" + strconv.Itoa(colorDepth) +
		"&colors=" + colorsParam

	return display, requestURI
}

func initWifiStack(logger, driverLogger *slog.Logger) *cywnet.Stack {
	logger.Info("Initializing WiFi...")
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = driverLogger
	stack, err := cywnet.NewConfiguredPicoWithStack(config.WifiSSID, config.WifiPassword, devcfg, cywnet.StackConfig{
		Hostname:          "tiny-dash-pico",
		MaxActiveTCPPorts: 1,
	})
	util.RequireNoError(err, "Wifi initialization failed")
	logger.Info("✓ WiFi hardware initialized")
	return stack
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(pollTime)
		}
	}
}

func parseServerAddr() netip.AddrPort {
	serverAddr := config.APIHost + ":" + config.APIPort
	svAddr, err := netip.ParseAddrPort(serverAddr)
	util.RequireNoError(err, "parsing server address")
	return svAddr
}

func buildHTTPRequest(requestURI string, svAddr netip.AddrPort) []byte {
	var reqHdr httpraw.HeaderV1
	reqHdr.SetMethod("GET")
	reqHdr.SetRequestTarget(requestURI)
	reqHdr.SetProtocol("HTTP/1.1")
	reqHdr.Set("Host", svAddr.Addr().String())
	reqHdr.Set("Connection", "close")
	reqBytes, err := reqHdr.AppendRequest(nil)
	util.RequireNoError(err, "building HTTP request")
	return reqBytes
}

func fetchImage(rstack *xnet.StackRetrying, lstack *xnet.StackAsync, svAddr netip.AddrPort, reqBytes, framebuf, headerBuf, rxBuf, txBuf []byte, expectedSize int, logger *slog.Logger) error {
	var conn tcp.Conn
	err := conn.Configure(tcp.ConnConfig{
		RxBuf:             rxBuf,
		TxBuf:             txBuf,
		TxPacketQueueSize: 3,
		RWBackoff:         func(uint) time.Duration { return pollTime },
	})
	if err != nil {
		return fmt.Errorf("conn configure: %w", err)
	}

	lport := uint16(lstack.Prand32()>>17) + 1024
	err = rstack.DoDialTCP(&conn, lport, svAddr, connTimeout, 3)
	if err != nil {
		closeConn(&conn)
		return fmt.Errorf("tcp dial: %w", err)
	}

	_, err = conn.Write(reqBytes)
	if err != nil {
		closeConn(&conn)
		return fmt.Errorf("write request: %w", err)
	}

	// Parse response headers
	var respHdr httpraw.HeaderV1
	respHdr.Reset(nil, 0)
	headerDeadline := time.Now().Add(10 * time.Second)
	conn.SetReadDeadline(headerDeadline)
	headerParsed := false

	for time.Now().Before(headerDeadline) {
		n, _ := conn.Read(headerBuf)
		if n > 0 {
			respHdr.ReadFromBytes(headerBuf[:n])
			needMore, parseErr := respHdr.TryParse(true)
			if parseErr != nil && !needMore {
				closeConn(&conn)
				return fmt.Errorf("header parse: %w", parseErr)
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
		closeConn(&conn)
		return fmt.Errorf("response header timeout")
	}

	statusCode, statusText := respHdr.Status()
	if string(statusCode) != "200" {
		closeConn(&conn)
		return fmt.Errorf("unexpected HTTP status: %s %s", statusCode, statusText)
	}

	// Stream body into framebuffer
	bodyReceived := 0

	initialBody, bodyErr := respHdr.Body()
	if bodyErr != nil {
		closeConn(&conn)
		return fmt.Errorf("get body: %w", bodyErr)
	}
	if len(initialBody) > 0 {
		n := copy(framebuf[bodyReceived:], initialBody)
		bodyReceived += n
	}

	bodyDeadline := time.Now().Add(30 * time.Second)
	conn.SetReadDeadline(bodyDeadline)
	for bodyReceived < expectedSize && time.Now().Before(bodyDeadline) {
		n, _ := conn.Read(framebuf[bodyReceived:])
		if n > 0 {
			bodyReceived += n
		} else {
			time.Sleep(pollTime)
		}
	}

	closeConn(&conn)

	if bodyReceived != expectedSize {
		return fmt.Errorf("incomplete image data: received %d, expected %d", bodyReceived, expectedSize)
	}

	return nil
}

func closeConn(conn *tcp.Conn) {
	conn.Close()
	for i := 0; i < 50 && !conn.State().IsClosed(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	conn.Abort()
}
