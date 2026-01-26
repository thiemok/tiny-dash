package main

import (
	"fmt"
	"log/slog"
	"machine"
	"net/netip"
	"strings"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/examples/cywnet"
	"github.com/soypat/lneto/http/httpraw"
	"github.com/soypat/lneto/tcp"
	"github.com/thiemok/tiny-dash/picoDevice/internal/config"
	"github.com/thiemok/tiny-dash/util/pkg"
)

const connTimeout = 5 * time.Second
const tcpbufsize = 2030 // MTU - ethhdr - iphdr - tcphdr

func main() {
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	time.Sleep(5 * time.Second)

	logger.Info("tiny-dash picoDevice Starting...")

	//logger.Info("Initializing hardware...")
	//hardware, err := adapters.NewPico2PicoToPiHardware()
	//util.RequireNoError(err, "Failed to initialize hardware")
	//logger.Info("✓ Hardware initialized")
	//
	//logger.Info("Detecting display via EEPROM...")
	//display, err := inky.Auto(*hardware)
	//util.RequireNoError(err, "Failed to detect display")
	//logger.Info("✓ Display detected")
	//
	//width := display.Width()
	//height := display.Height()
	//colorDepth := display.ColorDepth()
	//supportedColors := display.SupportedColors()
	//
	//logger.Info("Display Information", "resolution", fmt.Sprintf("%d x %d", width, height), "colorDepth", colorDepth, "colors", len(supportedColors))

	logger.Info("Initializing WiFi...")
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = logger
	cystack, err := cywnet.NewConfiguredPicoWithStack(config.WifiSSID, config.WifiPassword, devcfg, cywnet.StackConfig{
		Hostname:    "tiny-dash-pico",
		MaxTCPPorts: 1,
	})
	util.RequireNoError(err, "Wifi initialization failed")
	logger.Info("✓ WiFi hardware initialized")

	// Start packet processing loop
	go loopForeverStack(cystack)

	logger.Info("Configuring network via DHCP...")
	dhcpResults, err := cystack.SetupWithDHCP(cywnet.DHCPConfig{})
	util.RequireNoError(err, "DHCP failed")
	logger.Info("✓ DHCP complete", "ip address", dhcpResults.AssignedAddr.String())

	logger.Info("Configuration", "api host", config.APIHost+":"+config.APIPort, "refresh interval", config.RefreshInterval)

	logger.Info("Starting main refresh loop...")

	//// Main refresh loop
	//refreshCount := 0
	//for {
	//	refreshCount++
	//	logger.Info("---")
	//	logger.Info("Refresh cycle", "count", refreshCount, "time", time.Now().Format("2006-01-02 15:04:05"))
	//
	//	// Build color list
	//	colorStrs := make([]string, len(supportedColors))
	//	for i, c := range supportedColors {
	//		colorStrs[i] = fmt.Sprintf("%d", byte(c))
	//	}
	//	colorParam := strings.Join(colorStrs, ",")
	//
	//	// Fetch image from API
	//	logger.Info("Fetching image from API...")
	//	imageData, err := fetchImage(logger, cystack, width, height, colorDepth, colorParam)
	//	if err != nil {
	//		logger.Error("Failed to fetch image", "error", err)
	//		logger.Info("Keeping previous image")
	//	} else {
	//		// Validate size
	//		expectedSize := len(display.Buffer())
	//		if len(imageData) != expectedSize {
	//			logger.Error("Received wrong size", "received bytes", len(imageData), "expected bytes", expectedSize)
	//			logger.Info("Keeping previous image")
	//		} else {
	//			logger.Info("✓ Image received", "bytes", len(imageData))
	//
	//			// Copy to framebuffer
	//			copy(display.Buffer(), imageData)
	//
	//			// Update display
	//			logger.Info("Updating display (this may take 30-40 seconds)...")
	//			if err := display.Update(); err != nil {
	//				logger.Error("Display update failed", "error", err)
	//			} else {
	//				logger.Info("✓ Display updated successfully")
	//			}
	//		}
	//	}
	//
	//	// Wait for next refresh
	//	logger.Info("Waiting until next refresh...", "seconds", config.RefreshInterval)
	//	time.Sleep(time.Duration(config.RefreshInterval) * time.Second)
	//}
}

func fetchImage(logger *slog.Logger, cystack *cywnet.Stack, width, height, colorDepth int, colors string) ([]byte, error) {
	path := fmt.Sprintf("/api/dashboard/image?width=%d&height=%d&colorDepth=%d&colors=%s",
		width, height, colorDepth, colors)

	serverAddr := config.APIHost + ":" + config.APIPort
	logger.Info("Connecting to server", "address", serverAddr)

	svAddr, err := netip.ParseAddrPort(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("parsing server address: %w", err)
	}

	stack := cystack.LnetoStack()
	const pollTime = 5 * time.Millisecond
	rstack := stack.StackRetrying(pollTime)

	// Configure TCP connection
	var conn tcp.Conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, tcpbufsize),
		TxBuf:             make([]byte, tcpbufsize),
		TxPacketQueueSize: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("conn configure: %w", err)
	}

	// Build HTTP request using httpraw.Header
	var hdr httpraw.Header
	hdr.SetMethod("GET")
	hdr.SetRequestURI(path)
	hdr.SetProtocol("HTTP/1.1")
	hdr.Set("Host", svAddr.Addr().String())
	hdr.Set("Connection", "close")
	reqbytes, err := hdr.AppendRequest(nil)
	if err != nil {
		return nil, fmt.Errorf("building HTTP request: %w", err)
	}

	// Random local port
	lport := uint16(stack.Prand32()>>17) + 1024

	// Dial TCP with retries
	err = rstack.DoDialTCP(&conn, lport, svAddr, connTimeout, 3)
	if err != nil {
		closeConn(&conn)
		return nil, fmt.Errorf("tcp dial failed: %w", err)
	}

	// Send the HTTP request
	_, err = conn.Write(reqbytes)
	if err != nil {
		closeConn(&conn)
		return nil, fmt.Errorf("writing request: %w", err)
	}

	// Read response with multiple attempts
	rxBuf := make([]byte, 0, 8192)
	readBuf := make([]byte, tcpbufsize)
	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		n, err := conn.Read(readBuf)
		if n > 0 {
			rxBuf = append(rxBuf, readBuf[:n]...)
		}
		if err != nil || n == 0 {
			break
		}
	}

	closeConn(&conn)

	if len(rxBuf) == 0 {
		return nil, fmt.Errorf("no response received")
	}

	// Parse HTTP response to extract body
	body, err := parseHTTPResponse(rxBuf)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func parseHTTPResponse(data []byte) ([]byte, error) {
	// Find the end of headers (double CRLF)
	idx := -1
	for i := 0; i <= len(data)-4; i++ {
		if data[i] == '\r' && data[i+1] == '\n' && data[i+2] == '\r' && data[i+3] == '\n' {
			idx = i + 4
			break
		}
	}

	if idx == -1 {
		return nil, fmt.Errorf("invalid HTTP response: no header delimiter found")
	}

	// Check status line
	statusLine := string(data[:idx])
	if !strings.Contains(statusLine, "200 OK") && !strings.Contains(statusLine, "HTTP/1.1 200") {
		return nil, fmt.Errorf("HTTP request failed: %s", strings.Split(statusLine, "\r\n")[0])
	}

	// Extract body
	body := data[idx:]
	return body, nil
}

func closeConn(conn *tcp.Conn) {
	conn.Close()
	for i := 0; i < 50 && !conn.State().IsClosed(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	conn.Abort()
}

func loopForeverStack(stack *cywnet.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
