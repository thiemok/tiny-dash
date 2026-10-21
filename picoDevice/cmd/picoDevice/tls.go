package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"
	"unsafe"

	"github.com/soypat/lcrypto/x509"
	"github.com/thiemok/tiny-dash/netkit/pkg/apiurl"
	"github.com/thiemok/tiny-dash/netkit/pkg/bufshare"
	"github.com/thiemok/tiny-dash/netkit/pkg/wallclock"
	"github.com/thiemok/tiny-dash/tinytls"
	"github.com/thiemok/tiny-dash/tinytls/drbg"
	"github.com/thiemok/tiny-dash/tinytls/roots"
	util "github.com/thiemok/tiny-dash/util/pkg"
)

// A validated handshake takes ~255 ms on the Pico 2 W: X25519 plus four RSA
// verifications (THI-28). The timeout only bounds a server that stops
// answering.
const (
	handshakeTimeout = 10 * time.Second
	// tlsMaxSignatureChecks bounds the chain building a hostile server can make
	// the device do; the Let's Encrypt chain needs three.
	tlsMaxSignatureChecks = 16
	// trngSeedBlocks seed the DRBG with twice its 256-bit security in raw
	// TRNG samples, rounded up to whole samples.
	trngSeedBlocks = (2*drbg.MinSeed + trngBlockSize - 1) / trngBlockSize
	// tlsRecordOverhead is a TLS 1.3 record's header, content type and
	// AES-GCM tag.
	tlsRecordOverhead = 5 + 1 + 16
)

var (
	errClockNotSynced   = errors.New("wall clock not synced; certificates cannot be validated")
	errHandshakeTimeout = errors.New("TLS handshake timeout")
)

// tlsClient is the https state, allocated once at startup and reused by every
// fetch: tinytls.Conn holds a 16.6 KiB record buffer, as the server sends
// full 16 KiB records and ignores max_fragment_length. The 8 KiB handshake
// buffer and, behind it, the Verifier's ~18.4 KiB scratch are borrowed from
// the framebuffer through fb, which the body overwrites anyway once the
// handshake is done: neither tinytls nor the Verifier reads them after
// Handshake returns, and fetch only shows a complete body. A display smaller
// than that leaves tinytls and the Verifier to allocate their own.
type tlsClient struct {
	conn     tinytls.Conn
	verifier x509.Verifier
	chain    x509.Chain
	cfg      tinytls.Config
	clock    *wallclock.Clock

	fb          *bufshare.Buffer // nil if tinytls owns its handshake buffer.
	fbUser      bufshare.User
	handshaking bool

	trng   trng
	rng    drbg.Reader
	sample trngBlock

	handshakeTook time.Duration
}

// newTLSClient seeds the DRBG from the TRNG and configures certificate
// validation against the embedded roots on the wall clock, which reads 0 and
// so validates nothing until NTP has set it. Without entropy no handshake can
// be secure, so a TRNG failure halts the device.
func newTLSClient(logger *slog.Logger, apiURL apiurl.URL, clock *wallclock.Clock, fb *bufshare.Buffer) *tlsClient {
	t := &tlsClient{clock: clock}
	start := time.Now()
	t.trng.init()
	for range trngSeedBlocks {
		err := t.trng.read(&t.sample)
		if err != nil {
			logger.Error("TRNG unusable", "error", err, "healthFailures", t.trng.healthFailures)
		}
		util.RequireNoError(err, "seeding the TLS DRBG from the TRNG")
		t.rng.Seed(t.sample[:])
	}
	clear(t.sample[:])
	logger.LogAttrs(context.Background(), slog.LevelInfo, "✓ TRNG seeded the TLS DRBG",
		slog.Duration("took", time.Since(start)), slog.Int("healthFailures", t.trng.healthFailures))

	t.chain = roots.ISRG()
	vcfg := x509.VerifierConfig{
		Roots:              &t.chain,
		Nanotime:           clock.Nanotime,
		MaxSignatureChecks: tlsMaxSignatureChecks,
	}
	t.cfg = tinytls.Config{ServerName: apiURL.Host, Rand: &t.rng, Verifier: &t.verifier}
	if vcfg.Scratch = verifierScratch(fb.Bytes()); vcfg.Scratch != nil {
		t.fb = fb
		t.fbUser = bufshare.User{Name: "tls handshake", Idle: t.handshakeIdle}
		t.cfg.HandshakeBuf = fb.Bytes()[:tinytls.HandshakeBufSize]
		// Configure parses the roots in the scratch.
		util.RequireNoError(t.fb.Acquire(&t.fbUser), "borrowing the framebuffer")
	} else {
		logger.Warn("framebuffer too small to lend; TLS allocates its own handshake buffer and verifier scratch", "framebuffer", len(fb.Bytes()))
	}
	t.handshaking = true
	err := t.verifier.Configure(vcfg)
	t.handshaking = false
	util.RequireNoError(err, "configuring certificate validation")
	return t
}

// verifierScratch places a Verifier's scratch in fb behind the handshake
// buffer, or returns nil if it does not fit. The scratch's pointers lead only
// into itself, fb and memory the tlsClient holds, so it does not matter that
// the GC does not scan fb, a pointer-free []byte.
func verifierScratch(fb []byte) *x509.VerifierScratch {
	if len(fb) < tinytls.HandshakeBufSize {
		return nil
	}
	// The heap starts wherever .bss ends, 4-byte aligned, so fb is not always
	// as aligned as the scratch's uint64s need.
	const align = unsafe.Alignof(x509.VerifierScratch{})
	start := uintptr(unsafe.Pointer(&fb[0])) + tinytls.HandshakeBufSize
	off := tinytls.HandshakeBufSize + (align-start%align)%align
	if uintptr(len(fb)) < off+unsafe.Sizeof(x509.VerifierScratch{}) {
		return nil
	}
	return (*x509.VerifierScratch)(unsafe.Pointer(&fb[off]))
}

// handshake runs the TLS handshake over rw, whose deadlines it sets. It mixes
// a fresh TRNG sample into the DRBG first, when one is ready.
func (t *tlsClient) handshake(rw io.ReadWriter, setDeadline func(time.Time) error) error {
	if !t.clock.Synced() {
		return errClockNotSynced
	}
	if t.trng.ready() && t.trng.read(&t.sample) == nil {
		t.rng.Seed(t.sample[:])
		clear(t.sample[:])
	}
	if t.fb != nil {
		// Fails while the framebuffer holds an image not yet shown.
		if err := t.fb.Acquire(&t.fbUser); err != nil {
			return err
		}
	}
	setDeadline(time.Now().Add(handshakeTimeout))
	start := time.Now()
	t.handshaking = true
	err := t.conn.Handshake(rw, t.cfg)
	t.handshaking = false
	t.handshakeTook = time.Since(start)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return errHandshakeTimeout
	}
	return err
}

func (t *tlsClient) handshakeIdle() bool { return !t.handshaking }

// isVerifyError matches the unwrapped error of tinytls.Conn.Handshake; unlike
// errors.As, the type assertion does not heap-allocate its target.
func isVerifyError(err error) bool {
	_, ok := err.(tinytls.VerifyError)
	return ok
}
