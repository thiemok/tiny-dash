package main

import (
	"device/rp"
	"machine"
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
	idleExit()
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

// The panel's 192 KB image goes out over SPI1 by DMA in ~1.8 s at 1 MHz while
// the CPU only waits, so it can run slow too. SPI1's divider then has to
// shrink by the same factor as clk_peri, so the panel still sees exactly the
// same SCK.

const xoscHz = 12_000_000

var panelSCR uint32 // SPI1's serial clock rate field at full speed

// panelBulkWrite is the inky common.BulkWrite hook. It leaves the clocks
// alone if the current divider has no exact equivalent at 12 MHz.
func panelBulkWrite(start bool) {
	const (
		pos = rp.SPI0_SSPCR0_SCR_Pos
		msk = rp.SPI0_SSPCR0_SCR_Msk
	)
	spi := rp.SPI1
	if !start {
		if clockIsSlow {
			spi.SSPCR0.ReplaceBits(panelSCR<<pos, msk, 0)
			clockFast()
		}
		return
	}
	if clockIsSlow || debugBuild {
		return
	}
	cpsr := spi.SSPCPSR.Get()
	scr := (spi.SSPCR0.Get() & msk) >> pos
	// clk_peri cycles per SCK period at full speed, and the same at 12 MHz.
	cycles := uint64(cpsr) * uint64(scr+1) * xoscHz
	full := uint64(machine.CPUFrequency())
	if cpsr == 0 || cycles%full != 0 || (cycles/full)%uint64(cpsr) != 0 {
		return
	}
	slowSCR := (cycles/full)/uint64(cpsr) - 1
	if slowSCR > msk>>pos {
		return
	}
	panelSCR = scr
	clockSlow()
	spi.SSPCR0.ReplaceBits(uint32(slowSCR)<<pos, msk, 0)
}
