package main

import (
	"device/rp"
	"encoding/binary"
	"encoding/hex"
	"machine"
	"time"

	"github.com/thiemok/tiny-dash/netkit/pkg/refresh"
)

// Between fetches the CYW43439 is powered off rather than kept joined: joined
// and dozing in PM2 it still draws ~32 mA, the board without it ~12 mA. A
// second cyw43439 Init after a power cycle fails (SPI test afff0fff), so the
// device reboots instead of reconnecting, through the boot path that always
// works. The watchdog scratch registers, which survive a watchdog reset,
// carry what the next boot needs.

const (
	// wakeLead is how long before the next fetch is due the device reboots:
	// boot, Init, Join and DHCP take ~6 s.
	wakeLead = 6 * time.Second
	// minDoze is the shortest wait worth powering WiFi off for; below it the
	// rejoin costs more than the idle it saves.
	minDoze = 15 * time.Second

	carryMagic     = 0x7d5b0000
	carryMagicMask = 0xffff0000
	carryETag      = 1 << 0
	carryPressed   = 1 << 1
)

// carry is the state handed from one boot to the next.
type carry struct {
	valid   bool
	pressed bool
	hasETag bool
	etag    [refresh.ETagLen]byte
	// shownAge is how long ago the image on the panel was drawn.
	shownAge time.Duration
}

// loadCarry reads and clears what the previous boot left.
func loadCarry() (c carry) {
	s0 := rp.WATCHDOG.SCRATCH0.Get()
	rp.WATCHDOG.SCRATCH0.Set(0)
	if s0&carryMagicMask != carryMagic {
		return c
	}
	c.valid = true
	c.pressed = s0&carryPressed != 0
	c.hasETag = s0&carryETag != 0
	var raw [refresh.ETagLen / 2]byte
	binary.LittleEndian.PutUint32(raw[0:], rp.WATCHDOG.SCRATCH1.Get())
	binary.LittleEndian.PutUint32(raw[4:], rp.WATCHDOG.SCRATCH2.Get())
	hex.Encode(c.etag[:], raw[:])
	c.shownAge = time.Duration(rp.WATCHDOG.SCRATCH3.Get()) * time.Second
	return c
}

// rebootWith stores the state for the next boot and resets the device.
func rebootWith(etagSlot []byte, pressed bool, shownAge time.Duration) {
	s0 := uint32(carryMagic)
	var raw [refresh.ETagLen / 2]byte
	if _, err := hex.Decode(raw[:], etagSlot); err == nil {
		s0 |= carryETag
	}
	if pressed {
		s0 |= carryPressed
	}
	rp.WATCHDOG.SCRATCH1.Set(binary.LittleEndian.Uint32(raw[0:]))
	rp.WATCHDOG.SCRATCH2.Set(binary.LittleEndian.Uint32(raw[4:]))
	rp.WATCHDOG.SCRATCH3.Set(uint32(shownAge / time.Second))
	rp.WATCHDOG.SCRATCH0.Set(s0)
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 1})
	machine.Watchdog.Start()
	for {
	}
}

// wifiOff powers the CYW43439 down for the rest of this boot.
func (w *wifiLink) wifiOff() {
	w.pause()
	machine.GPIO23.Low() // WL_REG_ON
	w.off = true
}
