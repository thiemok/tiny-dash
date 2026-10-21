package main

import (
	"device/rp"
	"encoding/binary"
	"errors"
	"time"
)

// The RP2350 TRNG (Arm TrustZone TRNG, RP2350 datasheet 12.12) samples a
// free-running ring oscillator into a 192-bit EHR register, with von Neumann
// decorrelation and the continuous (CRNGT) and autocorrelation health tests
// enabled. TinyGo's machine.GetRNG does not use it: it is the pico-sdk lwIP
// jitter loop over a single ROSC bit, not meant for keys, and crypto/rand has
// no rp2350 source at all (its Reader is nil).
//
// The samples are not full entropy, so they seed a drbg.Reader rather than
// being used directly: at boot with twice the DRBG's security in raw bits
// (trngSeedBlocks), and with one fresh sample before every handshake.

const (
	trngBlockSize = 24
	trngTimeout   = 500 * time.Millisecond
	// trngMaxFailures failed health tests in a row mean the source is broken
	// rather than unlucky.
	trngMaxFailures = 8
	trngErrBits     = rp.TRNG_RNG_ISR_VN_ERR | rp.TRNG_RNG_ISR_CRNGT_ERR | rp.TRNG_RNG_ISR_AUTOCORR_ERR
)

var (
	errTRNGTimeout = errors.New("TRNG produced no sample")
	errTRNGHealth  = errors.New("TRNG failed its health tests")
)

type trngBlock [trngBlockSize]byte

type trng struct {
	// healthFailures counts discarded samples since boot.
	healthFailures int
}

func (t *trng) init() {
	rp.RESETS.RESET.ClearBits(rp.RESETS_RESET_TRNG)
	for !rp.RESETS.RESET_DONE.HasBits(rp.RESETS_RESET_DONE_TRNG) {
	}
	t.start()
}

func (t *trng) start() {
	rp.TRNG.TRNG_DEBUG_CONTROL.Set(0) // No test bypassed.
	rp.TRNG.RNG_ICR.Set(0xffffffff)
	rp.TRNG.RND_SOURCE_ENABLE.Set(1)
}

// ready reports whether a sample can be read without waiting.
func (t *trng) ready() bool {
	return rp.TRNG.TRNG_VALID.HasBits(rp.TRNG_TRNG_VALID_EHR_VALID) && !rp.TRNG.RNG_ISR.HasBits(trngErrBits)
}

// read waits for the next 192-bit sample that passed the health tests, and
// starts collecting the one after. A failed test stops the TRNG; it is reset
// and the sample discarded.
func (t *trng) read(dst *trngBlock) error {
	deadline := time.Now().Add(trngTimeout)
	failures := 0
	for {
		if isr := rp.TRNG.RNG_ISR.Get(); isr&trngErrBits != 0 {
			t.healthFailures++
			if failures++; failures == trngMaxFailures {
				return errTRNGHealth
			}
			rp.TRNG.TRNG_SW_RESET.Set(1)
			t.start()
			continue
		}
		if rp.TRNG.TRNG_VALID.HasBits(rp.TRNG_TRNG_VALID_EHR_VALID) {
			break
		}
		if time.Now().After(deadline) {
			return errTRNGTimeout
		}
		time.Sleep(time.Millisecond)
	}
	le := binary.LittleEndian
	le.PutUint32(dst[0:], rp.TRNG.EHR_DATA0.Get())
	le.PutUint32(dst[4:], rp.TRNG.EHR_DATA1.Get())
	le.PutUint32(dst[8:], rp.TRNG.EHR_DATA2.Get())
	le.PutUint32(dst[12:], rp.TRNG.EHR_DATA3.Get())
	le.PutUint32(dst[16:], rp.TRNG.EHR_DATA4.Get())
	// Reading the last word starts the next collection.
	le.PutUint32(dst[20:], rp.TRNG.EHR_DATA5.Get())
	rp.TRNG.RNG_ICR.Set(rp.TRNG_RNG_ICR_EHR_VALID)
	return nil
}
