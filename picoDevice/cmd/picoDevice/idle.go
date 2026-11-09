package main

import (
	"device/arm"
	"device/rp"
	"runtime/interrupt"
)

// While the device only waits, for the next fetch or for the panel's BUSY
// pin, idleDeep trims what clockSlow leaves running: on top of the 12 MHz
// crystal clock, the RP2350 otherwise still runs PLL_USB with clk_usb and
// clk_adc at 48 MHz, the ring oscillator, and every clock tree during WFE.
// idleKnobs selects how many of the steps below apply (diagnostic builds
// measure them one by one); the reboot after the wait undoes all of them,
// and idleExit does when the wait ends without one.
var idleKnobs = 4

const (
	knobUSBPLL   = 1 // clk_usb, clk_adc, clk_hstx and PLL_USB off
	knobSysDiv   = 2 // clk_sys = clk_ref / 12 (1 MHz)
	knobROSC     = 3 // ring oscillator off
	knobSleepGat = 4 // clock-gated sleep: only what wakes the core runs in WFE
)

// idleSysDiv divides clk_ref (12 MHz) down for clk_sys while idle. TIMER0
// counts TICKS from clk_ref, so time keeping is unaffected; clk_peri follows
// clk_sys, so no SPI, I2C or UART transfer may run while idle.
const idleSysDiv = 12

// sleepAwake lists the clocks that stay on in clock-gated sleep: the bus to
// reach them, the timer that ends the wait, the GPIO block whose interrupts
// latch button presses, the watchdog, the crystal and the power manager.
const (
	sleepAwake0 = rp.CLOCKS_SLEEP_EN0_CLK_SYS_CLOCKS | rp.CLOCKS_SLEEP_EN0_CLK_SYS_BUSFABRIC |
		rp.CLOCKS_SLEEP_EN0_CLK_SYS_BUSCTRL | rp.CLOCKS_SLEEP_EN0_CLK_SYS_IO |
		rp.CLOCKS_SLEEP_EN0_CLK_SYS_PADS | rp.CLOCKS_SLEEP_EN0_CLK_SYS_SIO |
		rp.CLOCKS_SLEEP_EN0_CLK_SYS_RESETS | rp.CLOCKS_SLEEP_EN0_CLK_SYS_PSM |
		rp.CLOCKS_SLEEP_EN0_CLK_SYS_POWMAN | rp.CLOCKS_SLEEP_EN0_CLK_REF_POWMAN |
		rp.CLOCKS_SLEEP_EN0_CLK_SYS_ACCESSCTRL
	sleepAwake1 = rp.CLOCKS_SLEEP_EN1_CLK_SYS_TIMER0 | rp.CLOCKS_SLEEP_EN1_CLK_SYS_TICKS |
		rp.CLOCKS_SLEEP_EN1_CLK_REF_TICKS | rp.CLOCKS_SLEEP_EN1_CLK_SYS_WATCHDOG |
		rp.CLOCKS_SLEEP_EN1_CLK_SYS_XOSC | rp.CLOCKS_SLEEP_EN1_CLK_SYS_XIP |
		rp.CLOCKS_SLEEP_EN1_CLK_SYS_SYSINFO | rp.CLOCKS_SLEEP_EN1_CLK_SYS_SYSCFG |
		rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM0 | rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM1 |
		rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM2 | rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM3 |
		rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM4 | rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM5 |
		rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM6 | rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM7 |
		rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM8 | rp.CLOCKS_SLEEP_EN1_CLK_SYS_SRAM9
)

const (
	auxClocks = rp.CLOCKS_CLK_USB_CTRL_ENABLE // same bit in the ADC and HSTX controls
	pllOff    = rp.PLL_SYS_PWR_PD | rp.PLL_SYS_PWR_VCOPD | rp.PLL_SYS_PWR_POSTDIVPD
)

var (
	idleDeepOn               bool
	savedSleep0, savedSleep1 uint32
)

// idleDeep enters the deep idle state; it requires clockSlow.
func idleDeep() {
	if idleDeepOn || !clockIsSlow || idleKnobs == 0 {
		return
	}
	mask := interrupt.Disable()
	if idleKnobs >= knobUSBPLL {
		rp.CLOCKS.CLK_USB_CTRL.ClearBits(auxClocks)
		rp.CLOCKS.CLK_ADC_CTRL.ClearBits(auxClocks)
		rp.CLOCKS.CLK_HSTX_CTRL.ClearBits(auxClocks)
		rp.PLL_USB.PWR.SetBits(pllOff)
	}
	if idleKnobs >= knobSysDiv {
		rp.CLOCKS.CLK_SYS_DIV.Set(idleSysDiv << rp.CLOCKS_CLK_SYS_DIV_INT_Pos)
	}
	if idleKnobs >= knobROSC {
		rp.ROSC.CTRL.ReplaceBits(rp.ROSC_CTRL_ENABLE_DISABLE<<rp.ROSC_CTRL_ENABLE_Pos, rp.ROSC_CTRL_ENABLE_Msk, 0)
	}
	if idleKnobs >= knobSleepGat {
		savedSleep0, savedSleep1 = rp.CLOCKS.SLEEP_EN0.Get(), rp.CLOCKS.SLEEP_EN1.Get()
		rp.CLOCKS.SLEEP_EN0.Set(sleepAwake0)
		rp.CLOCKS.SLEEP_EN1.Set(sleepAwake1)
		arm.SCB.SCR.SetBits(arm.SCB_SCR_SLEEPDEEP)
	}
	idleDeepOn = true
	interrupt.Restore(mask)
}

// idleExit undoes idleDeep; clockFast calls it first.
func idleExit() {
	if !idleDeepOn {
		return
	}
	mask := interrupt.Disable()
	if idleKnobs >= knobSleepGat {
		arm.SCB.SCR.ClearBits(arm.SCB_SCR_SLEEPDEEP)
		rp.CLOCKS.SLEEP_EN0.Set(savedSleep0)
		rp.CLOCKS.SLEEP_EN1.Set(savedSleep1)
	}
	if idleKnobs >= knobROSC {
		rp.ROSC.CTRL.ReplaceBits(rp.ROSC_CTRL_ENABLE_ENABLE<<rp.ROSC_CTRL_ENABLE_Pos, rp.ROSC_CTRL_ENABLE_Msk, 0)
		for !rp.ROSC.STATUS.HasBits(rp.ROSC_STATUS_STABLE) {
		}
	}
	if idleKnobs >= knobSysDiv {
		rp.CLOCKS.CLK_SYS_DIV.Set(1 << rp.CLOCKS_CLK_SYS_DIV_INT_Pos)
	}
	if idleKnobs >= knobUSBPLL {
		rp.PLL_USB.PWR.ClearBits(rp.PLL_SYS_PWR_PD | rp.PLL_SYS_PWR_VCOPD)
		for !rp.PLL_USB.CS.HasBits(rp.PLL_SYS_CS_LOCK) {
		}
		rp.PLL_USB.PWR.ClearBits(rp.PLL_SYS_PWR_POSTDIVPD)
		rp.CLOCKS.CLK_USB_CTRL.SetBits(auxClocks)
		rp.CLOCKS.CLK_ADC_CTRL.SetBits(auxClocks)
		rp.CLOCKS.CLK_HSTX_CTRL.SetBits(auxClocks)
	}
	idleDeepOn = false
	interrupt.Restore(mask)
}
