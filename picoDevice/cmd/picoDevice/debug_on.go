//go:build picodebug

package main

import "github.com/thiemok/tiny-dash/picoDevice/internal/config"

// debugBuild compiles in the measurement and fault-injection code used to
// verify the firmware on hardware. Build with -tags=picodebug, e.g.
// `pnpm nx run picoDevice:build:debug`.
const debugBuild = true

// Hardware test knobs from .env, see .env.example; 0 disables them.
const (
	wifiTestFailJoins        = config.WifiTestFailJoins
	wifiTestOutageAfterCycle = config.WifiTestOutageAfterCycle
	wifiTestOutageFailJoins  = config.WifiTestOutageFailJoins
	ntpTestSyncInterval      = config.NTPTestSyncInterval
	memTestGC                = config.MemTestGC
)
