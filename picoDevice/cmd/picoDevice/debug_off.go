//go:build !picodebug

package main

const debugBuild = false

// Release builds ignore the hardware test knobs in .env.
const (
	wifiTestFailJoins        = 0
	wifiTestOutageAfterCycle = 0
	wifiTestOutageFailJoins  = 0
	ntpTestSyncInterval      = 0
	memTestGC                = false
)

// simulateOutage is a debug-only fault injection; see wifi_debug.go.
func (*wifiLink) simulateOutage(int) {}
