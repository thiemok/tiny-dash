//go:build !tinygo

package tinytls

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	stdx509 "crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/soypat/lcrypto/x509"
	"github.com/soypat/lneto/crypto/tlsraw"
	"github.com/thiemok/tiny-dash/tinytls/roots"
)

// These tests run tinytls against crypto/tls servers over loopback TCP.

const testName = "tiny.test"

type testPKI struct {
	root   []byte
	server tls.Certificate
}

func newPKI(t testing.TB, leafKey crypto.Signer, notAfter time.Time) testPKI {
	t.Helper()
	return newPKIWithRoot(t, ecKey(t), leafKey, notAfter)
}

func newPKIWithRoot(t testing.TB, rootKey, leafKey crypto.Signer, notAfter time.Time) testPKI {
	t.Helper()
	now := time.Now()
	rootTmpl := &stdx509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "tinytls test root"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              stdx509.KeyUsageCertSign,
	}
	rootDER, err := stdx509.CreateCertificate(rand.Reader, rootTmpl, rootTmpl, rootKey.Public(), rootKey)
	if err != nil {
		t.Fatal(err)
	}
	rootCert, _ := stdx509.ParseCertificate(rootDER)
	leafTmpl := &stdx509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: testName},
		DNSNames:     []string{testName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     stdx509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []stdx509.ExtKeyUsage{stdx509.ExtKeyUsageServerAuth},
	}
	leafDER, err := stdx509.CreateCertificate(rand.Reader, leafTmpl, rootCert, leafKey.Public(), rootKey)
	if err != nil {
		t.Fatal(err)
	}
	return testPKI{root: rootDER, server: tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}}
}

func ecKey(t testing.TB) crypto.Signer {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func serverConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
}

// serve reads one request and answers it with size bytes, then sends
// close_notify. A client close_notify after the request is not an error.
func serve(conn net.Conn, cfg *tls.Config, size int) error {
	srv := tls.Server(conn, cfg)
	defer srv.Close()
	buf := make([]byte, 512)
	n, err := srv.Read(buf)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(buf[:n], []byte("GET")) {
		return errors.New("unexpected request")
	}
	if _, err = srv.Write(bytes.Repeat([]byte{0xa5}, size)); err != nil {
		return err
	}
	return srv.CloseWrite()
}

// tcpPair returns a loopback TCP connection: unlike net.Pipe it buffers, so
// the server can send session tickets while the client writes its request.
func tcpPair(t testing.TB) (cli, srv net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cli, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if srv, err = ln.Accept(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close(); srv.Close() })
	return cli, srv
}

func verifierFor(t testing.TB, chain *x509.Chain, now func() time.Time) *x509.Verifier {
	t.Helper()
	v := new(x509.Verifier)
	err := v.Configure(x509.VerifierConfig{Roots: chain, Nanotime: func() int64 { return now().UnixNano() }})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestHandshakeAgainstGoServer(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]crypto.Signer{"ecdsa-p256": ecKey(t), "rsa-2048": rsaKey} {
		t.Run(name, func(t *testing.T) {
			pki := newPKI(t, key, time.Now().Add(time.Hour))
			chain := x509.Chain{pki.root}
			v := verifierFor(t, &chain, time.Now)
			var c Conn // Reused across handshakes as on the device.
			for i := range 3 {
				const size = 100_000 // Spans several 16 KiB records.
				cli, srv := tcpPair(t)
				errc := make(chan error, 1)
				go func() { errc <- serve(srv, serverConfig(pki.server), size) }()
				err := c.Handshake(cli, Config{ServerName: testName, Rand: rand.Reader, Verifier: v})
				if err != nil {
					t.Fatalf("handshake %d: %v", i, err)
				}
				if _, err = c.Write([]byte("GET / HTTP/1.1\r\n\r\n")); err != nil {
					t.Fatal(err)
				}
				got, err := io.ReadAll(&c)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != size || got[0] != 0xa5 || got[size-1] != 0xa5 {
					t.Fatalf("got %d bytes", len(got))
				}
				if err = c.Close(); err != nil {
					t.Fatal(err)
				}
				if err = <-errc; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestVerificationFailures(t *testing.T) {
	key := ecKey(t)
	pki := newPKI(t, key, time.Now().Add(time.Hour))
	expired := newPKI(t, key, time.Now().Add(time.Minute))
	other := newPKI(t, key, time.Now().Add(time.Hour))
	for _, tc := range []struct {
		name   string
		server tls.Certificate
		root   []byte
		sni    string
		now    func() time.Time
	}{
		{"wrong name", pki.server, pki.root, "other.test", time.Now},
		{"unknown root", pki.server, other.root, testName, time.Now},
		{"expired", expired.server, expired.root, testName, func() time.Time { return time.Now().Add(time.Hour) }},
		{"clock unset", pki.server, pki.root, testName, func() time.Time { return time.Unix(0, 0) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := x509.Chain{tc.root}
			cli, srv := tcpPair(t)
			errc := make(chan error, 1)
			go func() { errc <- serve(srv, serverConfig(tc.server), 1) }()
			var c Conn
			err := c.Handshake(cli, Config{ServerName: tc.sni, Rand: rand.Reader, Verifier: verifierFor(t, &chain, tc.now)})
			var ve VerifyError
			if !errors.As(err, &ve) {
				t.Fatalf("want VerifyError, got %v", err)
			}
			// The server sees the bad_certificate alert.
			if err = <-errc; err == nil || !strings.Contains(err.Error(), "bad certificate") {
				t.Fatalf("server error %v, want bad certificate alert", err)
			}
		})
	}
}

func TestServerAlerts(t *testing.T) {
	pki := newPKI(t, ecKey(t), time.Now().Add(time.Hour))
	chain := x509.Chain{pki.root}
	v := verifierFor(t, &chain, time.Now)
	tls12 := serverConfig(pki.server)
	tls12.MinVersion, tls12.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
	p256 := serverConfig(pki.server)
	p256.CurvePreferences = []tls.CurveID{tls.CurveP256}
	for _, tc := range []struct {
		name string
		cfg  *tls.Config
		want error
	}{
		{"server tls 1.2 only", tls12, PeerAlertError(tlsraw.AlertProtocolVersion)},
		// No group in common: Go answers handshake_failure rather than HelloRetryRequest.
		{"server wants p-256 only", p256, PeerAlertError(tlsraw.AlertHandshakeFailure)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli, srv := tcpPair(t)
			go serve(srv, tc.cfg, 1)
			var c Conn
			err := c.Handshake(cli, Config{ServerName: testName, Rand: rand.Reader, Verifier: v})
			if err != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// TestClientCloseNotify checks the server reads a clean EOF after the client
// closes in the middle of a response.
func TestClientCloseNotify(t *testing.T) {
	pki := newPKI(t, ecKey(t), time.Now().Add(time.Hour))
	chain := x509.Chain{pki.root}
	cli, srv := tcpPair(t)
	errc := make(chan error, 1)
	go func() {
		s := tls.Server(srv, serverConfig(pki.server))
		_, err := s.Read(make([]byte, 64))
		if err == nil {
			_, err = s.Read(make([]byte, 64))
		}
		errc <- err
	}()
	var c Conn
	if err := c.Handshake(cli, Config{ServerName: testName, Rand: rand.Reader, Verifier: verifierFor(t, &chain, time.Now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("GET")); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != io.EOF {
		t.Fatalf("server read %v, want EOF from close_notify", err)
	}
}

// recorder captures what the server sends.
type recorder struct {
	net.Conn
	got bytes.Buffer
}

func (r *recorder) Read(b []byte) (int, error) {
	n, err := r.Conn.Read(b)
	r.got.Write(b[:n])
	return n, err
}

// replay plays a recorded server flight back to a client whose Rand
// reproduces the recorded ClientHello.
type replay struct{ r bytes.Reader }

func (p *replay) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *replay) Write(b []byte) (int, error) { return len(b), nil }

// recordFlight records a validated handshake and a body of size bytes from a
// crypto/tls server, made with countingRand.
func recordFlight(t testing.TB, key crypto.Signer, size int) (flight []byte, v *x509.Verifier) {
	t.Helper()
	pki := newPKI(t, key, time.Now().Add(time.Hour))
	chain := x509.Chain{pki.root}
	at := time.Now()
	v = verifierFor(t, &chain, func() time.Time { return at })
	cli, srv := tcpPair(t)
	errc := make(chan error, 1)
	go func() { errc <- serve(srv, serverConfig(pki.server), size) }()
	rec := &recorder{Conn: cli}
	var c Conn
	if err := c.Handshake(rec, Config{ServerName: testName, Rand: &countingRand{}, Verifier: v}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("GET / HTTP/1.1\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, &c); err != nil {
		t.Fatal(err)
	}
	c.Close()
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	return rec.got.Bytes(), v
}

// TestReuseDoesNotAllocate replays a recorded connection on a reused Conn
// with a borrowed handshake buffer: after the first handshake, a validated
// handshake, the body and Close allocate nothing, and the buffer is left alone
// once Handshake returns.
func TestReuseDoesNotAllocate(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const size = 50_000
	flight, v := recordFlight(t, rsaKey, size)
	var c Conn
	var p replay
	var rnd countingRand
	hbuf := make([]byte, HandshakeBufSize)
	cfg := Config{ServerName: testName, Rand: &rnd, Verifier: v, HandshakeBuf: hbuf}
	req := []byte("GET / HTTP/1.1\r\n\r\n")
	body := make([]byte, size)
	run := func() {
		p.r.Reset(flight)
		rnd = countingRand{}
		if err := c.Handshake(&p, cfg); err != nil {
			t.Fatal(err)
		}
		for i := range hbuf {
			hbuf[i] = 0xee
		}
		if _, err := c.Write(req); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(&c, body); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Read(body[:1]); err != io.EOF {
			t.Fatalf("got %v, want EOF after close_notify", err)
		}
		c.Close()
		for _, b := range hbuf {
			if b != 0xee {
				t.Fatal("HandshakeBuf written after Handshake returned")
			}
		}
	}
	run()
	if c.ownHbuf != nil {
		t.Fatal("Conn allocated a handshake buffer despite Config.HandshakeBuf")
	}
	if allocs := testing.AllocsPerRun(5, run); allocs != 0 {
		t.Fatalf("%v allocations per connection, want 0", allocs)
	}
}

// TestLive fetches from a real server, e.g.
// TINYTLS_LIVE_ADDR=192.0.2.20:443 TINYTLS_LIVE_NAME=dashboard.example.com
func TestLive(t *testing.T) {
	addr, name := os.Getenv("TINYTLS_LIVE_ADDR"), os.Getenv("TINYTLS_LIVE_NAME")
	if addr == "" {
		t.Skip("TINYTLS_LIVE_ADDR not set")
	}
	chain := roots.ISRG()
	v := verifierFor(t, &chain, time.Now)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var c Conn
	start := time.Now()
	if err = c.Handshake(conn, Config{ServerName: name, Rand: rand.Reader, Verifier: v}); err != nil {
		t.Fatal(err)
	}
	t.Log("handshake", time.Since(start))
	req := "GET /api/hello HTTP/1.1\r\nHost: " + name + "\r\nConnection: close\r\n\r\n"
	if _, err = c.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(&c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "HTTP/1.1 200") || !strings.Contains(string(got), "Hello, World!") {
		t.Fatalf("unexpected response %q", got)
	}
}

// TestWriteReplayFixture records testdata/replay.bin for replay_test.go, which
// also runs under TinyGo: UPDATE_REPLAY_FIXTURE=1 go test -run TestWriteReplayFixture
func TestWriteReplayFixture(t *testing.T) {
	if os.Getenv("UPDATE_REPLAY_FIXTURE") == "" {
		t.Skip("UPDATE_REPLAY_FIXTURE not set")
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// RSA throughout like the deployed Let's Encrypt chain: lcrypto's ECDSA
	// verification allocates its curve points.
	pki := newPKIWithRoot(t, rsaKey, rsaKey, time.Now().Add(time.Hour))
	at := time.Now()
	chain := x509.Chain{pki.root}
	v := verifierFor(t, &chain, func() time.Time { return at })
	cli, srv := tcpPair(t)
	errc := make(chan error, 1)
	go func() { errc <- serve(srv, serverConfig(pki.server), replayBodySize) }()
	rec := &recorder{Conn: cli}
	var c Conn
	if err := c.Handshake(rec, Config{ServerName: testName, Rand: &countingRand{}, Verifier: v}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write(replayRequest); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, &c); err != nil {
		t.Fatal(err)
	}
	c.Close()
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	fx := replayFixture{root: pki.root, at: at.UnixNano(), flight: rec.got.Bytes()}
	if err := os.WriteFile("testdata/replay.bin", fx.marshal(), 0o644); err != nil {
		t.Fatal(err)
	}
}
