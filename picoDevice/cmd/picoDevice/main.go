package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"machine"
	"net/netip"
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
	"github.com/thiemok/tiny-dash/picoDevice/internal/config"
	util "github.com/thiemok/tiny-dash/util/pkg"
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr
const pollTime = 5 * time.Millisecond

func main() {
	logger := initLogger()
	time.Sleep(5 * time.Second)
	logger.Info("tiny-dash picoDevice Starting...")

	display, requestURI := initDisplay(logger)
	stack := initWifiStack(logger)

	go loopForeverStack(stack)

	// DHCP
	dhcpResults, err := stack.SetupWithDHCP(cywnet.DHCPConfig{})
	util.RequireNoError(err, "DHCP failed")
	logger.Info("✓ DHCP complete", "ip", dhcpResults.AssignedAddr.String())

	svAddr := parseServerAddr()
	reqBytes := buildHTTPRequest(requestURI, svAddr)

	// Pre-allocate buffers to avoid per-iteration allocation
	framebuf := display.Buffer()
	expectedSize := len(framebuf)
	headerBuf := make([]byte, 256)
	rxBuf := make([]byte, tcpbufsize)
	txBuf := make([]byte, tcpbufsize)

	lstack := stack.LnetoStack()
	rstack := lstack.StackRetrying(pollTime)
	refreshDuration := time.Duration(config.RefreshInterval) * time.Second

	logger.Info("Starting dashboard image fetch loop...", "uri", requestURI, "expectedBytes", expectedSize)

	for {
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

		time.Sleep(refreshDuration)
	}
}

func initLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
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

func initWifiStack(logger *slog.Logger) *cywnet.Stack {
	logger.Info("Initializing WiFi...")
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	stack, err := cywnet.NewConfiguredPicoWithStack(config.WifiSSID, config.WifiPassword, devcfg, cywnet.StackConfig{
		Hostname:    "tiny-dash-pico",
		MaxTCPPorts: 1,
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
	var reqHdr httpraw.Header
	reqHdr.SetMethod("GET")
	reqHdr.SetRequestURI(requestURI)
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
	var respHdr httpraw.Header
	respHdr.Reset(nil)
	headerDeadline := time.Now().Add(10 * time.Second)
	headerParsed := false
	firstRead := true

	for time.Now().Before(headerDeadline) {
		n, _ := conn.Read(headerBuf)
		if n > 0 {
			data := headerBuf[:n]
			if firstRead {
				// Strip "HTTP/x.x " protocol prefix — the lneto httpraw parser
				// expects response lines as "STATUS_CODE STATUS_TEXT", not
				// "HTTP/1.1 STATUS_CODE STATUS_TEXT".
				if spIdx := bytes.IndexByte(data, ' '); spIdx > 0 {
					data = data[spIdx+1:]
				}
				firstRead = false
			}
			respHdr.ReadFromBytes(data)
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
