package main

import (
	"device/arm"
	"device/rp"
	"runtime/interrupt"
	"runtime/volatile"
	"time"
	"unsafe"
)

// Between fetches the RP2350 powers its switched core down (POWMAN state
// P1.7: core, SRAM and XIP off) instead of idling: idle at 1 MHz and 0.90 V
// it still draws ~1 mA over the board. POWMAN's always-on timer, ticking
// from the low-power oscillator, powers it back up in time for the next
// fetch, or the refresh button does, through a level-low wake-up on its
// GPIO. Waking is a full boot, as after the watchdog reboot this replaces,
// so the state for the next boot travels in POWMAN's scratch registers,
// which survive both. The sequence follows pico-sdk's pico_low_power.

// refreshButtonGPIO is button A's pin, see inky's pico2_picotopi adapter.
const refreshButtonGPIO = 15

// wakeGPIO is the pin that wakes the device; the gp26wake rig build moves
// it.
var wakeGPIO uint32 = refreshButtonGPIO

// minDeepSleep is the shortest sleep worth powering down for; shorter waits
// idle as before.
const minDeepSleep = 3 * time.Second

const (
	nvicICER   = 0xE000E180 // interrupt clear-enable, 32 lines each
	nvicICPR   = 0xE000E280 // interrupt clear-pending
	sysTickCSR = 0xE000E010
)

// POWMAN registers have atomic set and clear aliases, like every RP2350
// peripheral, and only take writes that carry the password.
const (
	aliasSet = 0x2000
	aliasClr = 0x3000
)

func powmanAlias(r *volatile.Register32, off uintptr) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(r), off))
}

func powmanSetBits(r *volatile.Register32, bits uint32) {
	powmanAlias(r, aliasSet).Set(powmanPassword | bits)
}

func powmanClearBits(r *volatile.Register32, bits uint32) {
	powmanAlias(r, aliasClr).Set(powmanPassword | bits)
}

// aonMillis reads the always-on timer, in milliseconds.
func aonMillis() uint64 {
	hi := rp.POWMAN.READ_TIME_UPPER.Get()
	for {
		lo := rp.POWMAN.READ_TIME_LOWER.Get()
		next := rp.POWMAN.READ_TIME_UPPER.Get()
		if next == hi {
			return uint64(hi)<<32 | uint64(lo)
		}
		hi = next
	}
}

// lposcHz measures the low-power oscillator against clk_ref (the 12 MHz
// crystal) with frequency counter 0, to ~0.1 %: uncalibrated it is only
// nominally 32.768 kHz.
func lposcHz() uint32 {
	c := rp.CLOCKS
	for c.FC0_STATUS.HasBits(rp.CLOCKS_FC0_STATUS_RUNNING) {
	}
	c.FC0_REF_KHZ.Set(xoscHz / 1000)
	c.FC0_INTERVAL.Set(15) // ~32 ms, the longest
	c.FC0_MIN_KHZ.Set(0)
	c.FC0_MAX_KHZ.Set(0xffffffff)
	c.FC0_SRC.Set(rp.CLOCKS_FC0_SRC_FC0_SRC_LPOSC_CLKSRC)
	for !c.FC0_STATUS.HasBits(rp.CLOCKS_FC0_STATUS_DONE) {
	}
	r := c.FC0_RESULT.Get()
	khz := (r & rp.CLOCKS_FC0_RESULT_KHZ_Msk) >> rp.CLOCKS_FC0_RESULT_KHZ_Pos
	frac := r & rp.CLOCKS_FC0_RESULT_FRAC_Msk // 1/32 kHz
	return khz*1000 + frac*1000/32
}

// aonStart keeps the always-on timer running from the low-power oscillator,
// the only clock left in P1.7, with its tick calibrated now. A reset by RUN
// or power-on stops and zeroes it; wakes from P1.7 and watchdog reboots do
// not. It needs POWMAN readable, so not while idleDeep is in effect.
func aonStart() {
	hz := lposcHz()
	khzInt, khzFrac := hz/1000, (hz%1000)*65536/1000
	t := &rp.POWMAN.TIMER
	running := t.HasBits(rp.POWMAN_TIMER_RUN)
	if running && t.HasBits(rp.POWMAN_TIMER_USING_LPOSC) &&
		rp.POWMAN.LPOSC_FREQ_KHZ_INT.Get() == khzInt && rp.POWMAN.LPOSC_FREQ_KHZ_FRAC.Get() == khzFrac {
		return
	}
	var now uint64
	if running {
		now = aonMillis()
		powmanClearBits(t, rp.POWMAN_TIMER_RUN)
	}
	// Changing the tick means stopping the timer; set it back to where it
	// was so it keeps counting milliseconds since its start.
	rp.POWMAN.SET_TIME_63TO48.Set(powmanPassword | uint32(now>>48)&0xffff)
	rp.POWMAN.SET_TIME_47TO32.Set(powmanPassword | uint32(now>>32)&0xffff)
	rp.POWMAN.SET_TIME_31TO16.Set(powmanPassword | uint32(now>>16)&0xffff)
	rp.POWMAN.SET_TIME_15TO0.Set(powmanPassword | uint32(now)&0xffff)
	rp.POWMAN.LPOSC_FREQ_KHZ_INT.Set(powmanPassword | khzInt)
	rp.POWMAN.LPOSC_FREQ_KHZ_FRAC.Set(powmanPassword | khzFrac)
	powmanSetBits(t, rp.POWMAN_TIMER_USE_LPOSC)
	powmanSetBits(t, rp.POWMAN_TIMER_RUN)
	for !t.HasBits(rp.POWMAN_TIMER_USING_LPOSC) {
	}
}

// disableWakeups stops the alarm and the button from powering the core up
// again while it runs; pico-sdk does the same after a wake.
func disableWakeups() {
	powmanClearBits(&rp.POWMAN.PWRUP0, rp.POWMAN_PWRUP0_ENABLE)
	powmanClearBits(&rp.POWMAN.TIMER, rp.POWMAN_TIMER_PWRUP_ON_ALARM|rp.POWMAN_TIMER_ALARM_ENAB)
}

// wokeFromSleep reports whether this boot is a wake from P1.7 and whether
// the refresh button triggered it.
func wokeFromSleep() (fromSleep, byButton bool) {
	fromSleep = rp.POWMAN.CHIP_RESET.HasBits(rp.POWMAN_CHIP_RESET_HAD_SWCORE_PD)
	// LAST_SWCORE_PWRUP has a bit per source: bit 0 chip reset, bits 1-4
	// PWRUP0-3, bit 5 the debugger, bit 6 the timer alarm.
	return fromSleep, fromSleep && rp.POWMAN.LAST_SWCORE_PWRUP.HasBits(1<<1)
}

// deepSleep powers the switched core down for d, or until the refresh
// button is pressed, and only returns if POWMAN refused; the caller then
// waits as before. The carry must already be in POWMAN's scratch registers.
func deepSleep(d time.Duration) (refused string) {
	aonStart()
	until := aonMillis() + uint64(aonFromReal(d)/time.Millisecond)

	// The debugger's power-up request would otherwise hold the core on.
	powmanSetBits(&rp.POWMAN.DBG_PWRCFG, rp.POWMAN_DBG_PWRCFG_IGNORE)
	// Lets the regulator switch to its low-power mode.
	powmanSetBits(&rp.POWMAN.VREG_CTRL, rp.POWMAN_VREG_CTRL_UNLOCK)

	t := &rp.POWMAN.TIMER
	powmanClearBits(t, rp.POWMAN_TIMER_ALARM_ENAB)
	rp.POWMAN.ALARM_TIME_63TO48.Set(powmanPassword | uint32(until>>48)&0xffff)
	rp.POWMAN.ALARM_TIME_47TO32.Set(powmanPassword | uint32(until>>32)&0xffff)
	rp.POWMAN.ALARM_TIME_31TO16.Set(powmanPassword | uint32(until>>16)&0xffff)
	rp.POWMAN.ALARM_TIME_15TO0.Set(powmanPassword | uint32(until)&0xffff)
	powmanClearBits(t, rp.POWMAN_TIMER_ALARM)
	powmanSetBits(t, rp.POWMAN_TIMER_ALARM_ENAB)
	powmanSetBits(t, rp.POWMAN_TIMER_PWRUP_ON_ALARM)

	// The button pulls GPIO15 low; its pad keeps the pull-up and input
	// enable through the power-down.
	p := &rp.POWMAN.PWRUP0
	p.Set(powmanPassword | rp.POWMAN_PWRUP0_MODE_LEVEL<<8 | wakeGPIO) // DIRECTION 0: low
	powmanClearBits(p, rp.POWMAN_PWRUP0_STATUS)
	powmanSetBits(p, rp.POWMAN_PWRUP0_ENABLE)

	// The USB controller (serial in release builds) leaves its PHY powered
	// and un-isolated, ~0.4 mA through the sleep; a reset isolates it.
	rp.RESETS.RESET.SetBits(rp.RESETS_RESET_USBCTRL)

	// SRAM goes from off in P1.7 to on at the wake-up.
	powmanClearBits(&rp.POWMAN.SEQ_CFG, rp.POWMAN_SEQ_CFG_HW_PWRUP_SRAM0|rp.POWMAN_SEQ_CFG_HW_PWRUP_SRAM1)
	// Boot normally from flash.
	rp.POWMAN.BOOT0.Set(0)
	rp.POWMAN.BOOT1.Set(0)
	rp.POWMAN.BOOT2.Set(0)
	rp.POWMAN.BOOT3.Set(0)

	// Once the request is in, POWMAN may move the regulator to its
	// low-power (linear) mode before the core reaches WFI: at 150 MHz the
	// core then browns out and restarts at once (INTR.VREG_OUTPUT_LOW, no
	// LAST_SWCORE_PWRUP bit); from the 12 MHz crystal it sleeps.
	clockSlow()
	mask := interrupt.Disable()
	s := &rp.POWMAN.STATE
	powmanClearBits(s, rp.POWMAN_STATE_REQ_IGNORED)
	// REQ holds the inverted state; P1.7 has every domain off (0).
	s.Set(powmanPassword | 0xf<<4)
	switch {
	case s.HasBits(rp.POWMAN_STATE_REQ_IGNORED):
		refused = "request ignored (pending power-up, e.g. button held)"
	case s.HasBits(rp.POWMAN_STATE_BAD_SW_REQ):
		refused = "bad state request"
	default:
		for range 500 {
			if s.HasBits(rp.POWMAN_STATE_WAITING) {
				// POWMAN waits for the core to sleep, then powers it off.
				// A pending interrupt ends WFI even while masked, and a
				// core that wakes during the power-down restarts at once,
				// so nothing may be left to fire: no NVIC line enabled or
				// pending, no SysTick.
				for i := uintptr(0); i < 2; i++ {
					(*volatile.Register32)(unsafe.Pointer(nvicICER + 4*i)).Set(0xffffffff)
					(*volatile.Register32)(unsafe.Pointer(nvicICPR + 4*i)).Set(0xffffffff)
				}
				(*volatile.Register32)(unsafe.Pointer(uintptr(sysTickCSR))).Set(0)
				arm.SCB.SCR.SetBits(arm.SCB_SCR_SLEEPDEEP)
				for {
					arm.Asm("wfi")
				}
			}
		}
		refused = "STATE_WAITING never came up"
	}
	// Withdraw the request: back to P0.0, every domain on.
	s.Set(powmanPassword | 0<<4)
	interrupt.Restore(mask)
	disableWakeups()
	return refused
}

// lposcHz measures the low-power oscillator awake; asleep it ran ~0.2 %
// faster on the rig, so the always-on timer overstates time spent in P1.7
// (7 s early per hour). aonPPM is how much faster it counts than the wall
// clock, learned from NTP-synced boots (see calibrateAON) and kept in
// SCRATCH7 across sleeps; a reset by RUN or power-on starts over at 0.
var aonPPM int32

const (
	aonCalMagic = 0x5c000000
	aonCalMask  = 0xff000000
	aonCalBias  = 0x800000
	// aonMaxPPM bounds a believable error, aonMaxStep a believable change
	// of it. Each NTP sync is off by up to a few hundred milliseconds, so
	// a measurement over a short span is mostly noise: aonMinSpan.
	aonMaxPPM  = 100_000
	aonMaxStep = 5_000
	aonMinSpan = 5 * time.Minute
)

func loadAONCal() {
	if v := rp.POWMAN.SCRATCH7.Get(); v&aonCalMask == aonCalMagic {
		aonPPM = int32(v&^aonCalMask) - aonCalBias
	}
}

func storeAONCal() {
	rp.POWMAN.SCRATCH7.Set(aonCalMagic | uint32(aonPPM+aonCalBias)&^aonCalMask)
}

// aonToReal converts always-on timer time into a real duration.
func aonToReal(d time.Duration) time.Duration {
	return time.Duration(int64(d) * 1_000_000 / (1_000_000 + int64(aonPPM)))
}

// aonFromReal converts a real duration into always-on timer time.
func aonFromReal(d time.Duration) time.Duration {
	return d + time.Duration(int64(d)*int64(aonPPM)/1_000_000)
}

// calibrateAON compares how far the wall clock and the always-on timer moved
// since the previous boot stored the carry, once this boot's clock is
// synced, and folds the difference into aonPPM.
func calibrateAON(c carry, wallNow time.Time) (ppm int32, ok bool) {
	if !c.hasWall {
		return 0, false
	}
	real := time.Duration(uint32(wallNow.UnixMilli())-c.wallMillis) * time.Millisecond
	aon := time.Duration(uint32(aonMillis())-c.aonMillis) * time.Millisecond
	if real < aonMinSpan || real > 49*24*time.Hour {
		return 0, false
	}
	measured := int64(aon-real) * 1_000_000 / int64(real)
	if measured > aonMaxPPM || measured < -aonMaxPPM {
		return 0, false
	}
	if step := measured - int64(aonPPM); aonPPM != 0 && (step > aonMaxStep || step < -aonMaxStep) {
		return int32(measured), false
	}
	if aonPPM == 0 {
		aonPPM = int32(measured)
	} else {
		aonPPM = int32((3*int64(aonPPM) + measured) / 4)
	}
	storeAONCal()
	return int32(measured), true
}
