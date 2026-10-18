package main

import (
	"device/rp"
	"runtime/volatile"
	"time"
	"unsafe"
)

const (
	// cdcInBufCtl is the DPSRAM buffer control register of the USB-CDC data IN
	// endpoint (EP2 IN), see machine/machine_rp2_usb.go.
	cdcInBufCtlAddr = 0x50100000 + 0x80 + 2*8
	usbBufAvail     = 0x400
	usbWakePoll     = 50 * time.Millisecond
	usbWakeBackoff  = time.Second
)

// keepUSBHostAwake implements USB remote wakeup, which TinyGo 0.43 advertises
// in its configuration descriptor (bmAttributes 0xa0) but never signals.
// Linux autosuspends the device after 2s without serial traffic even while a
// reader has the port open; without a wakeup, the next IN packet then stays
// armed until the host resumes on its own (typically the next open/close), so
// serial output stops and any Write to machine.Serial blocks.
func keepUSBHostAwake() {
	inBufCtl := (*volatile.Register32)(unsafe.Pointer(uintptr(cdcInBufCtlAddr)))
	suspendedPolls := 0
	for {
		time.Sleep(usbWakePoll)
		if rp.USB.GetSIE_STATUS_SUSPENDED() == 0 {
			suspendedPolls = 0
			continue
		}
		suspendedPolls++
		// USB 2.0 7.1.7.7: the bus must have been idle for at least 5ms before
		// a device signals resume; two polls guarantee that.
		if suspendedPolls < 2 || !inBufCtl.HasBits(usbBufAvail) {
			continue
		}
		rp.USB.SIE_CTRL.SetBits(rp.USB_SIE_CTRL_RESUME)
		time.Sleep(usbWakeBackoff)
	}
}
