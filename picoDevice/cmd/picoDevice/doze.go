package main

import (
	"device/rp"
	"encoding/binary"
	"encoding/hex"
	"machine"
	"time"

	"github.com/soypat/cyw43439/whd"
	"github.com/thiemok/tiny-dash/netkit/pkg/refresh"
)

// Between fetches the CYW43439 is powered off rather than kept joined: joined
// and dozing in PM2 it still draws ~32 mA, the board without it ~12 mA. A
// second cyw43439 Init after a power cycle fails (SPI test afff0fff), so the
// device reboots instead of reconnecting, through the boot path that always
// works. POWMAN's scratch registers carry what the next boot needs.

const (
	// wakeLead is how long before the next fetch is due the device reboots
	// when it has not timed a wake yet, as after a cold boot.
	wakeLead = 6 * time.Second
	// A timed wake, from reset to the response, sets the lead of the next
	// reboot plus wakeMargin, within [minWakeLead, maxWakeLead]. A lead
	// longer than the wake makes the fetch early, and since the next one is
	// scheduled from the response, every cycle shorter than the interval.
	wakeMargin  = 500 * time.Millisecond
	minWakeLead = 2500 * time.Millisecond
	maxWakeLead = 8 * time.Second
	// minDoze is the shortest wait worth powering WiFi off for; below it the
	// rejoin costs more than the idle it saves.
	minDoze = 15 * time.Second
	// leaveTime lets the deauthentication frame go out before power-off.
	leaveTime = 50 * time.Millisecond

	carryMagic       = 0x7d5b0000
	carryMagicMask   = 0xffff0000
	carryETag        = 1 << 0
	carryPressed     = 1 << 1
	carryFailuresPos = 4
	carryFailuresMsk = 0xf << carryFailuresPos
	carryChannelPos  = 8
	carryChannelMsk  = 0xf << carryChannelPos
	maxFailures      = carryFailuresMsk >> carryFailuresPos
	maxChannel       = 14
)

// carry is the state handed from one boot to the next, through a watchdog
// reboot or a wake from deep sleep. It lives in POWMAN's scratch registers,
// which survive both (the watchdog's do not survive P1.7); a reset by RUN or
// power-on clears them.
type carry struct {
	valid    bool
	pressed  bool
	hasETag  bool
	channel  uint8
	failures uint
	etag     [refresh.ETagLen]byte
	// shownAge is how long ago the image on the panel was drawn.
	shownAge time.Duration
	// sinceResponse is how long ago the previous boot's last response came
	// in, if hasResponse; button presses wait for pressMinGap after it.
	sinceResponse time.Duration
	hasResponse   bool
	// fromSleep is set after a wake from deep sleep.
	fromSleep bool
	// aonMillis is the always-on timer when the carry was stored, and
	// wallMillis the wall clock then, if hasWall (it was synced); both
	// truncated to 32 bits. They calibrate the timer, see calibrateAON.
	aonMillis  uint32
	wallMillis uint32
	hasWall    bool
}

// carryAON marks SCRATCH4 (the always-on timer's milliseconds when the state
// was stored) and SCRATCH5 (its milliseconds at the last response) as set,
// carryWall SCRATCH6 (the wall clock's Unix milliseconds then).
const (
	carryAON  = 1 << 2
	carryWall = 1 << 3
)

// loadCarry reads and clears what the previous boot left.
func loadCarry() (c carry) {
	pm := rp.POWMAN
	loadAONCal()
	s0 := pm.SCRATCH0.Get()
	pm.SCRATCH0.Set(0)
	fromSleep, byButton := wokeFromSleep()
	if fromSleep {
		disableWakeups()
	}
	if s0&carryMagicMask != carryMagic {
		return c
	}
	c.valid = true
	c.fromSleep = fromSleep
	c.pressed = s0&carryPressed != 0 || byButton
	c.hasETag = s0&carryETag != 0
	c.channel = uint8(s0 & carryChannelMsk >> carryChannelPos)
	c.failures = uint(s0 & carryFailuresMsk >> carryFailuresPos)
	var raw [refresh.ETagLen / 2]byte
	binary.LittleEndian.PutUint32(raw[0:], pm.SCRATCH1.Get())
	binary.LittleEndian.PutUint32(raw[4:], pm.SCRATCH2.Get())
	hex.Encode(c.etag[:], raw[:])
	c.shownAge = time.Duration(pm.SCRATCH3.Get()) * time.Second
	if s0&carryAON != 0 {
		// 32 bits of milliseconds wrap after 49 days; differences do not.
		now := uint32(aonMillis())
		c.aonMillis = pm.SCRATCH4.Get()
		// Asleep the timer runs fast; aonPPM is loaded by now.
		c.shownAge += aonToReal(time.Duration(now-c.aonMillis) * time.Millisecond)
		c.sinceResponse = aonToReal(time.Duration(now-pm.SCRATCH5.Get()) * time.Millisecond)
		c.hasResponse = true
		if s0&carryWall != 0 {
			c.wallMillis, c.hasWall = pm.SCRATCH6.Get(), true
		}
	}
	return c
}

type handoff struct {
	etagSlot []byte
	pressed  bool
	shownAge time.Duration
	channel  uint8
	failures uint
}

// aonStamp lets the boot after a deep sleep add the time asleep to shownAge,
// hold back an early press (aonResponse is the always-on timer at the last
// response) and, when wallSynced, calibrate the timer against wall.
type aonStamp struct {
	aonResponse uint32
	wall        time.Time
	wallSynced  bool
}

func storeCarry(h handoff, stamp *aonStamp) {
	pm := rp.POWMAN
	s0 := uint32(carryMagic) | uint32(min(h.failures, maxFailures))<<carryFailuresPos
	if h.channel <= maxChannel {
		s0 |= uint32(h.channel) << carryChannelPos
	}
	var raw [refresh.ETagLen / 2]byte
	if _, err := hex.Decode(raw[:], h.etagSlot); err == nil {
		s0 |= carryETag
	}
	if h.pressed {
		s0 |= carryPressed
	}
	if stamp != nil {
		s0 |= carryAON
		pm.SCRATCH4.Set(uint32(aonMillis()))
		pm.SCRATCH5.Set(stamp.aonResponse)
		if stamp.wallSynced {
			s0 |= carryWall
			pm.SCRATCH6.Set(uint32(stamp.wall.UnixMilli()))
		}
	}
	pm.SCRATCH1.Set(binary.LittleEndian.Uint32(raw[0:]))
	pm.SCRATCH2.Set(binary.LittleEndian.Uint32(raw[4:]))
	pm.SCRATCH3.Set(uint32(h.shownAge / time.Second))
	pm.SCRATCH0.Set(s0)
}

func rebootWith(h handoff) {
	// POWMAN reads and writes nothing while idleDeep is in effect.
	idleExit()
	storeCarry(h, nil)
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 1})
	machine.Watchdog.Start()
	for {
	}
}

// wifiOff powers the CYW43439 down for the rest of this boot. It leaves the
// network first: an access point that still holds the association answers
// the next boot's authentication late, which kept every wake ~2 s longer.
func (w *wifiLink) wifiOff() {
	if w.off {
		return
	}
	w.pause()
	if w.up() {
		doIoctlSet(w.dev, whd.WLC_DISASSOC, whd.IF_STA, nil)
		// Give the firmware time to send the deauthentication frame.
		time.Sleep(leaveTime)
	}
	machine.GPIO23.Low() // WL_REG_ON
	w.off = true
}

// leadFor returns how long before a fetch to reboot, given how long this
// boot took from reset to its response, or 0 if it did not time one.
func leadFor(wake time.Duration) time.Duration {
	if wake == 0 {
		return wakeLead
	}
	return min(max(wake+wakeMargin, minWakeLead), maxWakeLead)
}
