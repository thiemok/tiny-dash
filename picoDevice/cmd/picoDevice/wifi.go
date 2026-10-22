package main

import (
	"context"
	"log/slog"
	"machine"
	"net/netip"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/lneto/x/xnet"
	"github.com/thiemok/tiny-dash/netkit/pkg/backoff"
	"github.com/thiemok/tiny-dash/picoDevice/internal/config"
)

const (
	hostname       = "tiny-dash-pico"
	dhcpTimeout    = 3 * time.Second
	dhcpRetries    = 3
	gatewayTimeout = 500 * time.Millisecond
	gatewayRetries = 4

	mtu = 1500
	// sendBufSize fits the largest frame EgressEthernet writes: the MTU plus
	// the 14 B Ethernet header, without a CRC since none is configured.
	// cyw43439.MaxFrameSize (2030 B) is the chip's limit, not the stack's.
	sendBufSize = mtu + 14
)

// testMissingSSID is joined instead of config.WifiSSID while a WIFI_TEST_*
// knob forces join failures, so the chip reports a genuine "no network" the
// way it does while the access point is down.
const testMissingSSID = "tiny-dash-test-missing-ap"

// wifiLink owns the CYW43439 and the lneto stack on top of it. It replaces
// cywnet.NewConfiguredPicoWithStack, which creates a new device and stack per
// call and whose SetupWithDHCP panics when the gateway does not answer ARP;
// neither can be retried.
type wifiLink struct {
	dev     *cyw43439.Device
	devcfg  cyw43439.Config
	stack   xnet.StackAsync
	sendbuf []byte
	onRecv  func([]byte)
	backoff backoff.Exponential
	logger  *slog.Logger

	// polling gates pollLoop, the only other user of dev and stack. A
	// goroutine that is paused across reconnects, rather than restarted,
	// cannot leak and does not allocate a new goroutine stack per attempt.
	polling atomic.Bool
	idle    atomic.Bool

	failJoins int
	inited    bool
	icmpReady bool

	// dnsFallback is preset on every stack reset; a DNS server from DHCP
	// replaces it.
	dnsFallback netip.Addr
	dnsServer   netip.Addr
	dnsSource   string

	// ipbuf and dnsbuf format addresses for the connect logs;
	// netip.Addr.String allocates on every reconnect.
	ipbuf  [len("255.255.255.255")]byte
	dnsbuf [len("255.255.255.255")]byte
}

func newWifiLink(logger *slog.Logger, devLogger *slog.Logger, dnsFallback netip.Addr) *wifiLink {
	devcfg := cyw43439.DefaultWifiConfig()
	devcfg.Logger = devLogger
	w := &wifiLink{
		dev:         cyw43439.NewPicoWDevice(),
		devcfg:      devcfg,
		sendbuf:     make([]byte, sendBufSize),
		backoff:     backoff.Exponential{Min: time.Second, Max: time.Minute},
		logger:      logger,
		failJoins:   wifiTestFailJoins,
		dnsFallback: dnsFallback,
	}
	w.onRecv = w.ingress
	go w.pollLoop()
	return w
}

// connect brings the link up, retrying with capped exponential backoff until
// it succeeds.
func (w *wifiLink) connect() {
	ctx := context.Background()
	var before, after runtime.MemStats
	if debugBuild {
		runtime.ReadMemStats(&before)
	}
	w.backoff.Reset()
	for attempt := 1; ; attempt++ {
		if debugBuild {
			// A flat goroutine count across reconnects shows pollLoop is reused.
			w.logger.LogAttrs(ctx, slog.LevelInfo, "wifi connecting",
				slog.Int("attempt", attempt), slog.Int("goroutines", runtime.NumGoroutine()))
		} else {
			w.logger.LogAttrs(ctx, slog.LevelInfo, "wifi connecting", slog.Int("attempt", attempt))
		}
		start := time.Now()
		addr, stage, err := w.tryConnect()
		if err == nil {
			if debugBuild {
				runtime.ReadMemStats(&after)
				w.logger.LogAttrs(ctx, slog.LevelInfo, "✓ WiFi connected",
					slog.Int("attempt", attempt),
					slog.String("ip", bytesString(addr.AppendTo(w.ipbuf[:0]))),
					slog.Duration("took", time.Since(start)),
					slog.Uint64("mallocs", after.Mallocs-before.Mallocs),
					slog.Uint64("bytes", after.TotalAlloc-before.TotalAlloc),
				)
			} else {
				w.logger.LogAttrs(ctx, slog.LevelInfo, "✓ WiFi connected",
					slog.Int("attempt", attempt),
					slog.String("ip", bytesString(addr.AppendTo(w.ipbuf[:0]))),
					slog.Duration("took", time.Since(start)),
				)
			}
			w.logDNSServer()
			return
		}
		wait := w.backoff.Next(rand32())
		w.logger.LogAttrs(ctx, slog.LevelWarn, "wifi connect failed",
			slog.Int("attempt", attempt),
			slog.String("stage", stage),
			slog.Any("error", err),
			slog.Duration("retryIn", wait),
		)
		time.Sleep(wait)
	}
}

func (w *wifiLink) tryConnect() (addr netip.Addr, stage string, err error) {
	w.pause()
	// A second Init fails the SPI read test (spi test failed:afff0fff), even
	// after Device.Reset and a second without power, so later attempts only
	// re-join, as the pico-sdk does after a failed join.
	if !w.inited {
		if err = w.dev.Init(w.devcfg); err != nil {
			return addr, "init", err
		}
		w.inited = true
		// The driver rejects (and logs) RecvEthHandle before the first Init.
		w.dev.RecvEthHandle(w.onRecv)
	}

	// The firmware rejects Join's ioctls while associated, so a link the chip
	// still reports up only gets a fresh stack and DHCP lease.
	if !w.up() {
		ssid := config.WifiSSID
		if debugBuild && w.failJoins > 0 {
			w.failJoins--
			ssid = testMissingSSID
			w.logger.LogAttrs(context.Background(), slog.LevelWarn, "test: joining missing SSID to force a join failure",
				slog.String("ssid", ssid), slog.Int("remaining", w.failJoins))
		}
		if err = w.dev.Join(ssid, cyw43439.JoinOptions{Passphrase: config.WifiPassword}); err != nil {
			return addr, "join", err
		}
	}
	mac, err := w.dev.HardwareAddr6()
	if err != nil {
		return addr, "mac", err
	}
	cfg := xnet.StackConfig{
		Hostname:          hostname,
		MaxActiveTCPPorts: 1,
		RandSeed:          int64(rand32()) | 1,
		HardwareAddress:   mac,
		MTU:               mtu,
		PassivePeers:      2,
		DNSServer:         w.dnsFallback,
	}
	// Reset allocates a new ICMP echo queue whenever ICMPQueueLimit is set
	// and leaves the ICMP client alone otherwise, so only the first Reset
	// sizes it; disabling ICMP clears it of echoes queued before the reset.
	if !w.icmpReady {
		cfg.ICMPQueueLimit = 2
	}
	err = w.stack.Reset(cfg)
	if err == nil {
		err = w.stack.EnableICMP(false)
	}
	if err == nil {
		err = w.stack.EnableICMP(true)
	}
	if err != nil {
		return addr, "stack reset", err
	}
	w.icmpReady = true
	w.resume()

	rstack := w.stack.StackRetrying(stackBackoff)
	results, err := rstack.DoDHCPv4([4]byte{}, dhcpTimeout, dhcpRetries)
	if err != nil {
		return addr, "dhcp", err
	}
	if err = w.stack.AssimilateDHCPResults(results); err != nil {
		return addr, "dhcp assimilate", err
	}
	switch {
	case len(results.DNSServers) > 0:
		w.dnsServer, w.dnsSource = results.DNSServers[0], "dhcp"
	case w.dnsFallback.IsValid():
		w.dnsServer, w.dnsSource = w.dnsFallback, "fallback"
	default:
		w.dnsServer, w.dnsSource = netip.Addr{}, ""
	}
	gatewayHW, err := rstack.DoResolveHardwareAddress6(results.Router, gatewayTimeout, gatewayRetries)
	if err != nil {
		return addr, "gateway arp", err
	}
	w.stack.SetGatewayHardwareAddr(gatewayHW)
	return netip.AddrFrom4(results.AssignedAddr4), "", nil
}

// logDNSServer reports the DNS server the last lease left the stack with.
// Without one, host names in API_URL cannot be resolved; IP literals still
// work.
func (w *wifiLink) logDNSServer() {
	ctx := context.Background()
	if !w.dnsServer.IsValid() {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "no DNS server: DHCP offered none and DNS_SERVER is unset")
		return
	}
	w.logger.LogAttrs(ctx, slog.LevelInfo, "dns server",
		slog.String("ip", bytesString(w.dnsServer.AppendTo(w.dnsbuf[:0]))),
		slog.String("source", w.dnsSource))
}

// up reports the link state the chip last signalled: LINK, DEAUTH and
// DISASSOC events drop it, and so does a failed join.
func (w *wifiLink) up() bool { return w.dev.IsLinkUp() }

func (w *wifiLink) pause() {
	w.polling.Store(false)
	for !w.idle.Load() {
		time.Sleep(pollTime)
	}
}

func (w *wifiLink) resume() {
	w.idle.Store(false)
	w.polling.Store(true)
}

func (w *wifiLink) pollLoop() {
	for {
		if !w.polling.Load() {
			w.idle.Store(true)
			time.Sleep(pollTime)
			continue
		}
		if !w.recvAndSend() {
			time.Sleep(pollTime)
		}
	}
}

// recvAndSend moves at most one frame each way and reports whether it did.
// Errors are dropped as in cywnet.Stack.RecvAndSend; lost frames surface as
// timeouts in the protocol that sent them.
func (w *wifiLink) recvAndSend() bool {
	gotRecv, _ := w.dev.PollOne()
	send, err := w.stack.EgressEthernet(w.sendbuf)
	if err != nil || send == 0 {
		return gotRecv
	}
	w.dev.SendEth(w.sendbuf[:send])
	return true
}

func (w *wifiLink) ingress(pkt []byte) { w.stack.IngressEthernet(pkt) }

// rand32 seeds the stack and jitters retries. The lneto PRNG cannot do this:
// it is only seeded by the StackAsync.Reset that the first attempt has yet
// to reach.
func rand32() uint32 {
	r, err := machine.GetRNG()
	if err != nil {
		return uint32(time.Now().UnixNano())
	}
	return r
}

// stackBackoff is cywnet.DefaultStackBackoff: sleeps between polls of DHCP
// and ARP, growing from 100 µs to 20 ms.
func stackBackoff(consecutive uint) time.Duration {
	const minWait, maxWait = 100 * time.Microsecond, 20 * time.Millisecond
	return min(minWait<<min(consecutive, 15), maxWait)
}
