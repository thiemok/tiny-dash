package tinytls

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/soypat/lcrypto/x509"
)

// replay.bin holds a validated RSA-2048 connection recorded from a crypto/tls
// server by TestWriteReplayFixture, so that the allocation check also runs
// under TinyGo, whose escape analysis differs from Go's.

const replayBodySize = 20_000

var replayRequest = []byte("GET / HTTP/1.1\r\n\r\n")

type replayFixture struct {
	root   []byte
	at     int64 // Verifier time, Unix nanoseconds.
	flight []byte
}

func (fx replayFixture) marshal() []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(len(fx.root)))
	b = append(b, fx.root...)
	b = binary.BigEndian.AppendUint64(b, uint64(fx.at))
	return append(b, fx.flight...)
}

func loadReplay(t *testing.T) replayFixture {
	b, err := os.ReadFile("testdata/replay.bin")
	if err != nil {
		t.Fatal(err)
	}
	n := binary.BigEndian.Uint32(b)
	b = b[4:]
	fx := replayFixture{root: b[:n]}
	b = b[n:]
	fx.at = int64(binary.BigEndian.Uint64(b))
	fx.flight = b[8:]
	return fx
}

// byteStream replays a server flight and discards what the client sends.
type byteStream struct {
	data []byte
	off  int
}

func (s *byteStream) Read(b []byte) (int, error) {
	if s.off == len(s.data) {
		return 0, io.EOF
	}
	n := copy(b, s.data[s.off:])
	s.off += n
	return n, nil
}

func (s *byteStream) Write(b []byte) (int, error) { return len(b), nil }

// TestReplayDoesNotAllocate: after the first handshake on a Conn, a
// validated handshake, the body and Close allocate nothing.
func TestReplayDoesNotAllocate(t *testing.T) {
	fx := loadReplay(t)
	chain := x509.Chain{fx.root}
	var v x509.Verifier
	if err := v.Configure(x509.VerifierConfig{Roots: &chain, Nanotime: func() int64 { return fx.at }}); err != nil {
		t.Fatal(err)
	}
	var c Conn
	var s byteStream
	var rnd countingRand
	cfg := Config{ServerName: "tiny.test", Rand: &rnd, Verifier: &v}
	body := make([]byte, replayBodySize)
	var runErr error
	run := func() {
		s = byteStream{data: fx.flight}
		rnd = countingRand{}
		if err := c.Handshake(&s, cfg); err != nil {
			runErr = err
			return
		}
		if _, err := c.Write(replayRequest); err != nil {
			runErr = err
			return
		}
		if _, err := io.ReadFull(&c, body); err != nil {
			runErr = err
			return
		}
		if _, err := c.Read(body[:1]); err != io.EOF {
			runErr = errors.New("no close_notify")
		}
		c.Close()
	}
	run()
	if runErr != nil {
		t.Fatal(runErr)
	}
	// A MemStats delta rather than testing.AllocsPerRun, whose count under
	// TinyGo disagrees with the delta around the same calls; TotalAlloc also
	// catches allocations a Boehm-collected host build does not count.
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 3 {
		run()
	}
	runtime.ReadMemStats(&after)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if mallocs, bytes := after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc; mallocs != 0 || bytes != 0 {
		t.Fatalf("3 connections allocated %d times, %d B; want 0", mallocs, bytes)
	}
}
