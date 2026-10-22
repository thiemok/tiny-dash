package main

import (
	"errors"
	"hash/crc32"
	"runtime"
	"runtime/volatile"
	"unsafe"
)

// lneto's ethernet package initialises its FCS table with
// crc32.MakeTable(crc32.IEEE) (ethernet/crc.go:13, unchanged up to fd69eebbb0c3).
// MakeTable runs through a sync.OnceFunc whose defer/recover TinyGo cannot
// evaluate at build time, so the init runs at boot and builds the 8 KiB
// slicing-by-8 table on the heap (Go 1.26 hash/crc32/crc32.go:113). Nothing
// here computes an IEEE CRC: the CYW43439 adds and checks the 802.11 FCS
// itself, so no Ethernet TX CRC is configured and lneto never checks one on
// receive.
//
// These reach into hash/crc32, not lneto. If Go renames either variable the
// link fails ("linker could not find symbol"); if it changes their meaning,
// the known-answer check in releaseCRC32Table stops the boot.

//go:extern hash/crc32.ieeeTable8
var crc32IEEETable8 uintptr

//go:extern hash/crc32.updateIEEE
var crc32UpdateIEEE func(crc uint32, p []byte) uint32

// releaseCRC32Table frees the slicing-by-8 table. IEEE updates fall back to a
// table-free bitwise CRC, so any later caller still gets correct checksums.
// Debug builds measure and return the heap bytes that freed, 0 once lneto
// stops building it; release builds return 0.
func releaseCRC32Table() (freed uint64, err error) {
	if crc32IEEETable8 == 0 {
		return 0, nil
	}
	var before, after runtime.MemStats
	if debugBuild {
		runtime.GC()
		runtime.ReadMemStats(&before)
	}
	crc32UpdateIEEE = updateIEEEBitwise
	// Volatile: when nothing reads the table, LLVM drops a plain store of the
	// global's initial value, but the GC still scans the global as a root.
	volatile.StoreUint32((*uint32)(unsafe.Pointer(&crc32IEEETable8)), 0)
	if debugBuild {
		// The refresh loop makes too little garbage to trigger a collection for
		// hours, so collect now for HeapAlloc to show the saving.
		runtime.GC()
		runtime.ReadMemStats(&after)
	}
	if crc32.ChecksumIEEE([]byte("123456789")) != 0xCBF43926 {
		return 0, errors.New("crc32 IEEE known-answer check failed after replacing hash/crc32 internals")
	}
	return before.HeapAlloc - after.HeapAlloc, nil
}

func updateIEEEBitwise(crc uint32, p []byte) uint32 {
	crc = ^crc
	for _, v := range p {
		crc ^= uint32(v)
		for range 8 {
			crc = crc>>1 ^ crc32.IEEE&-(crc&1)
		}
	}
	return ^crc
}
