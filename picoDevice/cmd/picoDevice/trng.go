package main

import (
	"device/rp"
	"encoding/binary"
	"errors"
	"machine"
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
	// trngSampleCount keeps SAMPLE_CNT1 at its reset value, the slowest and
	// best quality setting (datasheet 12.12.2): one ROSC bit every 65535
	// clk_sys cycles, 437 µs at 150 MHz. The datasheet's 20-25 make a sample
	// in ~2 ms at the cost of more failed health tests; the seed needs only
	// three samples, once per boot, and later samples are taken when ready.
	trngSampleCount = 0xffff
	// trngRawBitsPerBlock is the mean number of ROSC bits behind a sample: the
	// von Neumann decorrelator keeps one bit of each pair that differs, half
	// of them for an unbiased source. With trngSampleCount that is ~335 ms.
	trngRawBitsPerBlock = 4 * 8 * trngBlockSize
	// A failed autocorrelation test silently restarts the collection up to
	// three times before it raises AUTOCORR_ERR, so a healthy sample can take
	// four times the mean; trngTimeoutFactor allows twice that.
	trngTimeoutFactor = 8
	// trngMaxFailures failed health tests or trngMaxTimeouts timeouts in a row
	// mean the source is broken rather than unlucky.
	trngMaxFailures = 8
	trngMaxTimeouts = 3
	trngErrBits     = rp.TRNG_RNG_ISR_VN_ERR | rp.TRNG_RNG_ISR_CRNGT_ERR | rp.TRNG_RNG_ISR_AUTOCORR_ERR
)

var (
	errTRNGTimeout = errors.New("TRNG produced no sample")
	errTRNGHealth  = errors.New("TRNG failed its health tests")
)

type trngBlock [trngBlockSize]byte

type trng struct {
	timeout time.Duration
	// healthFailures counts the samples discarded since boot, timeouts those
	// that did not come in time.
	healthFailures int
	timeouts       int
}

func (t *trng) init() {
	t.timeout = trngTimeoutFactor * time.Duration(trngRawBitsPerBlock*trngSampleCount) * time.Second / time.Duration(machine.CPUFrequency())
	rp.RESETS.RESET.SetBits(rp.RESETS_RESET_TRNG)
	rp.RESETS.RESET.ClearBits(rp.RESETS_RESET_TRNG)
	for !rp.RESETS.RESET_DONE.HasBits(rp.RESETS_RESET_DONE_TRNG) {
	}
	t.start()
}

func (t *trng) start() {
	rp.TRNG.SAMPLE_CNT1.Set(trngSampleCount)
	rp.TRNG.TRNG_CONFIG.Set(0)        // Shortest ROSC chain, as the datasheet advises.
	rp.TRNG.TRNG_DEBUG_CONTROL.Set(0) // No test bypassed.
	rp.TRNG.RNG_ICR.Set(0xffffffff)
	rp.TRNG.RND_SOURCE_ENABLE.Set(1)
}

// restart resets the TRNG, which clears AUTOCORR_ERR and drops a collection
// in progress, and starts it again.
func (t *trng) restart() {
	rp.TRNG.TRNG_SW_RESET.Set(1)
	// The soft reset needs a fixed delay before the TRNG takes writes again;
	// the bootrom reads the register twice (datasheet 12.12.4.1).
	rp.TRNG.TRNG_SW_RESET.Get()
	rp.TRNG.TRNG_SW_RESET.Get()
	t.start()
}

// tryRead reads a sample if one is ready, without waiting. Like read, it
// restarts the TRNG after a failed health test, which stops it.
func (t *trng) tryRead(dst *trngBlock) bool {
	if rp.TRNG.RNG_ISR.HasBits(trngErrBits) {
		t.healthFailures++
		t.restart()
		return false
	}
	if !rp.TRNG.TRNG_VALID.HasBits(rp.TRNG_TRNG_VALID_EHR_VALID) {
		return false
	}
	t.take(dst)
	return true
}

// read waits for the next sample that passed the health tests, and starts
// collecting the one after. A failed test stops the TRNG and a sample may
// take far longer than usual (datasheet 12.12.4); either way the TRNG is
// restarted, up to trngMaxFailures and trngMaxTimeouts times in a row. The
// wait sleeps, so other goroutines keep running.
func (t *trng) read(dst *trngBlock) error {
	deadline := time.Now().Add(t.timeout)
	failures, timeouts := 0, 0
	for {
		if rp.TRNG.RNG_ISR.HasBits(trngErrBits) {
			t.healthFailures++
			if failures++; failures == trngMaxFailures {
				return errTRNGHealth
			}
			t.restart()
			deadline = time.Now().Add(t.timeout)
			continue
		}
		if rp.TRNG.TRNG_VALID.HasBits(rp.TRNG_TRNG_VALID_EHR_VALID) {
			break
		}
		if time.Now().After(deadline) {
			t.timeouts++
			if timeouts++; timeouts == trngMaxTimeouts {
				return errTRNGTimeout
			}
			t.restart()
			deadline = time.Now().Add(t.timeout)
			continue
		}
		time.Sleep(time.Millisecond)
	}
	t.take(dst)
	return nil
}

// autocorrStats returns how often the autocorrelation test ran and failed
// since the last reset.
func (t *trng) autocorrStats() (tries, fails int) {
	v := rp.TRNG.AUTOCORR_STATISTIC.Get()
	tries = int(v & rp.TRNG_AUTOCORR_STATISTIC_AUTOCORR_TRYS_Msk)
	fails = int(v & rp.TRNG_AUTOCORR_STATISTIC_AUTOCORR_FAILS_Msk >> rp.TRNG_AUTOCORR_STATISTIC_AUTOCORR_FAILS_Pos)
	return tries, fails
}

func (t *trng) take(dst *trngBlock) {
	le := binary.LittleEndian
	le.PutUint32(dst[0:], rp.TRNG.EHR_DATA0.Get())
	le.PutUint32(dst[4:], rp.TRNG.EHR_DATA1.Get())
	le.PutUint32(dst[8:], rp.TRNG.EHR_DATA2.Get())
	le.PutUint32(dst[12:], rp.TRNG.EHR_DATA3.Get())
	le.PutUint32(dst[16:], rp.TRNG.EHR_DATA4.Get())
	// Reading the last word starts the next collection.
	le.PutUint32(dst[20:], rp.TRNG.EHR_DATA5.Get())
	rp.TRNG.RNG_ICR.Set(rp.TRNG_RNG_ICR_EHR_VALID)
}
