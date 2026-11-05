//go:build uartlog

package main

import (
	"machine"

	"github.com/thiemok/tiny-dash/logkit"
)

// uartLog builds log to UART0 (TX on GP0) instead of USB, so the Debug Probe
// can read them while USB stays unplugged, as it must on the power rig.
// Build with -tags=uartlog; tools/build.sh then adds -serial=uart.
const uartLog = true

// initLogDrainer returns a ring buffer drainer to UART0 running on its own
// goroutine. The UART clock follows clk_sys, so output waits while the
// clocks run slow; it would come out at the wrong baud rate.
func initLogDrainer() *logkit.Drainer {
	drainer := &logkit.Drainer{
		Src:      logkit.NewRingWriter(logBuf[:]),
		Dst:      machine.Serial,
		Writable: func() bool { return !clockIsSlow },
	}
	go drainer.Run()
	return drainer
}
