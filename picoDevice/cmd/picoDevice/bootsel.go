package main

import (
	"context"
	"log/slog"
	"machine"
	"time"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
)

// The device sleeps with USB powered down almost all the time, so `tinygo
// flash` cannot reset it into the bootloader over USB serial. Holding button
// D while button A asks for a refresh does that instead: the LED blinks for
// bootloaderBlink, then the RP2350 reboots into its USB bootloader.
const (
	bootloaderButton = 3
	bootloaderBlink  = 5 * time.Second
	blinkPeriod      = 100 * time.Millisecond
	bootloaderSettle = 120 * time.Millisecond
)

// enterBootloaderIfHeld reboots into the USB bootloader when the bootloader
// button is held, and returns otherwise.
func enterBootloaderIfHeld(logger *slog.Logger, buttons *common.ButtonController, led *common.LEDController) {
	if buttons == nil || !buttons.IsPressed(bootloaderButton) {
		return
	}
	for start := time.Now(); time.Since(start) < bootloaderSettle; time.Sleep(buttonPollTime) {
		buttons.Poll()
	}
	if !buttons.IsPressed(bootloaderButton) {
		return
	}
	clockFast()
	logger.LogAttrs(context.Background(), slog.LevelInfo, "rebooting into the USB bootloader", slog.Duration("in", bootloaderBlink))
	for end := time.Now().Add(bootloaderBlink); time.Now().Before(end); time.Sleep(blinkPeriod) {
		if led != nil {
			led.Toggle()
		}
	}
	clearCarry()
	machine.EnterBootloader()
}
