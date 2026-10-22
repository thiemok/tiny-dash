//go:build picodebug

package main

// debugBuild compiles in the measurement and fault-injection code used to
// verify the firmware on hardware. Build with -tags=picodebug, e.g.
// `pnpm nx run picoDevice:build:debug`.
const debugBuild = true
