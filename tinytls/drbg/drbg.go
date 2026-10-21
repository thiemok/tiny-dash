// Package drbg turns a slow or raw entropy source into a fast
// cryptographically secure [io.Reader], e.g. for tinytls's Config.Rand on a
// microcontroller whose hardware RNG yields a few hundred bits per call.
//
// It is a SHA-256 hash DRBG in the spirit of NIST SP 800-90A Hash_DRBG, not a
// certified implementation:
//
//	Seed(e):  key = SHA-256(0x00 || key || e)
//	Read(p):  block i = SHA-256(0x01 || key || ctr+i), then
//	          key = SHA-256(0x02 || key || ctr)
//
// Every Read replaces the key, so output already returned cannot be recomputed
// from a later state. The Reader allocates nothing.
package drbg

import (
	"encoding/binary"
	"errors"

	"github.com/soypat/lcrypto/sha256"
)

// MinSeed is the entropy in bytes a Reader must be seeded with before it
// produces output. Seed it with more raw samples when they hold less than one
// bit of entropy per bit.
const MinSeed = 32

var ErrNotSeeded = errors.New("drbg: not seeded with MinSeed bytes")

// Reader is a DRBG. It is not safe for concurrent use.
type Reader struct {
	h      sha256.Digest
	key    [sha256.Size]byte
	ctr    uint64
	seeded int
	block  [sha256.Size]byte
	ctrBuf [8]byte
	// domain is the separator byte as a slice of r: a []byte literal passed
	// to the Digest would escape to the heap under TinyGo.
	domain [1]byte
}

// Seed mixes entropy into the state. It may be called at any time, e.g. with
// fresh hardware samples before every handshake.
func (r *Reader) Seed(entropy []byte) {
	r.h.Init()
	r.domain[0] = 0x00
	r.h.Write(r.domain[:])
	r.h.Write(r.key[:])
	r.h.Write(entropy)
	r.h.Sum(r.key[:0])
	r.seeded += len(entropy)
}

// Seeded reports whether the Reader has received MinSeed bytes of entropy.
func (r *Reader) Seeded() bool { return r.seeded >= MinSeed }

// Read fills p with output. It fails only if the Reader is not Seeded.
func (r *Reader) Read(p []byte) (int, error) {
	if !r.Seeded() {
		return 0, ErrNotSeeded
	}
	n := 0
	for n < len(p) {
		r.hashCounter(0x01, r.block[:0])
		r.ctr++
		n += copy(p[n:], r.block[:])
	}
	clear(r.block[:])
	r.hashCounter(0x02, r.key[:0])
	r.ctr++
	return n, nil
}

func (r *Reader) hashCounter(domain byte, dst []byte) {
	binary.BigEndian.PutUint64(r.ctrBuf[:], r.ctr)
	r.h.Init()
	r.domain[0] = domain
	r.h.Write(r.domain[:])
	r.h.Write(r.key[:])
	r.h.Write(r.ctrBuf[:])
	r.h.Sum(dst)
}
