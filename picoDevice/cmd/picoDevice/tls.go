package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/soypat/lcrypto/x509"
	"github.com/thiemok/tiny-dash/netkit/pkg/apiurl"
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
// full 16 KiB records, and the Verifier ~19 KiB of parse state.
type tlsClient struct {
	conn     tinytls.Conn
	verifier x509.Verifier
	chain    x509.Chain
	cfg      tinytls.Config
	clock    *wallclock.Clock

	trng   trng
	rng    drbg.Reader
	sample trngBlock

	handshakeTook time.Duration
}

// newTLSClient seeds the DRBG from the TRNG and configures certificate
// validation against the embedded roots on the wall clock, which reads 0 and
// so validates nothing until NTP has set it. Without entropy no handshake can
// be secure, so a TRNG failure halts the device.
func newTLSClient(logger *slog.Logger, apiURL apiurl.URL, clock *wallclock.Clock) *tlsClient {
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
	err := t.verifier.Configure(x509.VerifierConfig{
		Roots:              &t.chain,
		Nanotime:           clock.Nanotime,
		MaxSignatureChecks: tlsMaxSignatureChecks,
	})
	util.RequireNoError(err, "configuring certificate validation")
	t.cfg = tinytls.Config{ServerName: apiURL.Host, Rand: &t.rng, Verifier: &t.verifier}
	return t
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
	setDeadline(time.Now().Add(handshakeTimeout))
	start := time.Now()
	err := t.conn.Handshake(rw, t.cfg)
	t.handshakeTook = time.Since(start)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return errHandshakeTimeout
	}
	return err
}

func isVerifyError(err error) bool {
	var ve tinytls.VerifyError
	return errors.As(err, &ve)
}
