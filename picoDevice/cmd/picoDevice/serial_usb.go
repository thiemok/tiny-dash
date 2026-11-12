//go:build !uartlog

package main

import (
	"machine"

	"github.com/thiemok/tiny-dash/logkit"
)

// uartLog reports whether logs go to UART0 instead of USB, see serial_uart.go.
const uartLog = false

// initLogDrainer returns a ring buffer drainer to serial running on its own
// goroutine: TinyGo's USB-CDC Write spins until the host reads, so writing to
// machine.Serial directly can stall any goroutine that logs, including the
// network and display paths. Dropped bytes are reported by the drainer itself.
func initLogDrainer() *logkit.Drainer {
	drainer := &logkit.Drainer{
		Src:      logkit.NewRingWriter(logBuf[:]),
		Dst:      machine.Serial,
		Ready:    machine.Serial.DTR,
		Writable: (&usbWaker{}).Writable,
	}
	go drainer.Run()
	return drainer
}
