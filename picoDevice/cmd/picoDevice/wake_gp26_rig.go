//go:build gp26wake

package main

import "machine"

// The gp26wake build lets the power rig stand in for the refresh button:
// its logger pulls GP26 low through 1 kOhm (`powerlog.py press`), and the
// device wakes from deep sleep on that pin instead of GPIO15.
func init() {
	machine.GP26.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	wakeGPIO = 26
}
