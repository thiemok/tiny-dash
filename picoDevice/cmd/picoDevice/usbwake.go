package main

import (
	"device/rp"
	"time"
)

const (
	// USB 2.0 7.1.7.7: the bus must have been idle for at least 5ms before a
	// device signals resume.
	usbResumeMinIdle = 5 * time.Millisecond
	usbResumeBackoff = time.Second
)

// usbWaker implements USB remote wakeup, which TinyGo 0.43 advertises in its
// configuration descriptor (bmAttributes 0xa0) but never signals. Linux
// autosuspends the device after 2s without serial traffic even while a reader
// has the port open; without a wakeup, output then stays queued until the host
// resumes on its own (typically the next open/close) and any Write to
// machine.Serial blocks.
type usbWaker struct {
	suspendedAt time.Time
	resumedAt   time.Time
}

// Writable is a logkit.Drainer hook: it holds output back while the bus is
// suspended, so the drainer never blocks on a suspended host, and signals
// resume instead.
func (w *usbWaker) Writable() bool {
	if rp.USB.GetSIE_STATUS_SUSPENDED() == 0 {
		w.suspendedAt = time.Time{}
		return true
	}
	now := time.Now()
	if w.suspendedAt.IsZero() {
		w.suspendedAt = now
	}
	if now.Sub(w.suspendedAt) >= usbResumeMinIdle && now.Sub(w.resumedAt) >= usbResumeBackoff {
		rp.USB.SIE_CTRL.SetBits(rp.USB_SIE_CTRL_RESUME)
		w.resumedAt = now
	}
	return false
}
