package main

import (
	_ "unsafe"

	"github.com/soypat/cyw43439"
	"github.com/soypat/cyw43439/whd"
)

// doIoctlSet is the driver's internal ioctl call; it exports no way to leave
// a network. It does not lock the device, so the poller must be paused.
//
//go:linkname doIoctlSet (*github.com/soypat/cyw43439.Device).doIoctlSet
func doIoctlSet(d *cyw43439.Device, cmd whd.SDPCMCommand, iface whd.IoctlInterface, data []byte) error
