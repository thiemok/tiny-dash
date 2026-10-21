//go:build picodebug

package main

import (
	"device/arm"
	"unsafe"
)

// tinygoStackCanary is the word TinyGo writes at the bottom of every goroutine
// stack (internal/task.stackCanary, truncated to 32 bits).
const tinygoStackCanary = 0xb83bf575

// maxStackScan bounds the search for the canary below the stack pointer.
const maxStackScan = 64 << 10

// stackHeadroom returns how many bytes at the bottom of the calling
// goroutine's stack have never been written since it started: TinyGo allocates
// goroutine stacks zeroed with the canary at the bottom, so it is the run of
// zero words above the canary. The deepest frames may have stored zeros, so it
// can overstate the headroom by a few words. It returns 0 if no canary is found.
func stackHeadroom() uintptr {
	sp := arm.AsmFull("mov {}, sp", map[string]interface{}{}) &^ 3
	bottom := uintptr(0)
	for p := sp; p > sp-maxStackScan; p -= 4 {
		if *(*uint32)(unsafe.Pointer(p)) == tinygoStackCanary {
			bottom = p + 4
			break
		}
	}
	if bottom == 0 {
		return 0
	}
	p := bottom
	for p < sp && *(*uint32)(unsafe.Pointer(p)) == 0 {
		p += 4
	}
	return p - bottom
}
