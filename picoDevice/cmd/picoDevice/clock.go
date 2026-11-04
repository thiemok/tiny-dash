package main

import (
	"device/rp"
	"runtime/interrupt"
)

// While the CPU only waits (the panel's BUSY pin during a redraw, the wait
// for the next fetch) clk_sys runs from clk_ref, the 12 MHz crystal, and
// PLL_SYS is powered down: at 150 MHz the RP2350 draws several mA even in
// WFE. TIMER0 counts clk_ref ticks, so time keeping is unaffected; clk_peri
// follows clk_sys, so SPI, I2C and UART must not be used while slow, and
// machine.CPUFrequency still reports 150 MHz to anything configured then.
// Debug builds stay fast for USB, whose controller also runs on clk_sys.

const pllSysOff = rp.PLL_SYS_PWR_PD | rp.PLL_SYS_PWR_VCOPD | rp.PLL_SYS_PWR_POSTDIVPD

var clockIsSlow bool

func clockSlow() {
	if clockIsSlow || debugBuild {
		return
	}
	mask := interrupt.Disable()
	rp.CLOCKS.CLK_SYS_CTRL.ClearBits(rp.CLOCKS_CLK_SYS_CTRL_SRC_Msk)
	for rp.CLOCKS.CLK_SYS_SELECTED.Get() != 1<<rp.CLOCKS_CLK_SYS_CTRL_SRC_CLK_REF {
	}
	rp.PLL_SYS.PWR.SetBits(pllSysOff)
	interrupt.Restore(mask)
	clockIsSlow = true
}

func clockFast() {
	if !clockIsSlow {
		return
	}
	mask := interrupt.Disable()
	// The PLL keeps its dividers through power-down.
	rp.PLL_SYS.PWR.ClearBits(rp.PLL_SYS_PWR_PD | rp.PLL_SYS_PWR_VCOPD)
	for !rp.PLL_SYS.CS.HasBits(rp.PLL_SYS_CS_LOCK) {
	}
	rp.PLL_SYS.PWR.ClearBits(rp.PLL_SYS_PWR_POSTDIVPD)
	rp.CLOCKS.CLK_SYS_CTRL.SetBits(rp.CLOCKS_CLK_SYS_CTRL_SRC_CLKSRC_CLK_SYS_AUX)
	for rp.CLOCKS.CLK_SYS_SELECTED.Get() != 1<<rp.CLOCKS_CLK_SYS_CTRL_SRC_CLKSRC_CLK_SYS_AUX {
	}
	interrupt.Restore(mask)
	clockIsSlow = false
}
