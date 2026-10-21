package tinytls

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/soypat/lcrypto"
	"github.com/soypat/lcrypto/aesgcm"
	"github.com/soypat/lcrypto/x25519"
	"github.com/soypat/lneto/crypto/tlsraw"
)

// These tests drive the record layer and the handshake state machine with
// hand-made server records, so they run under TinyGo too.

// countingRand is a deterministic Rand: a handshake replays exactly when its
// server flight does.
type countingRand struct{ n byte }

func (r *countingRand) Read(p []byte) (int, error) {
	for i := range p {
		r.n++
		p[i] = r.n
	}
	return len(p), nil
}

// rwPair is a stream whose server side is in, and whose client output
// collects in out.
type rwPair struct {
	in  *bytes.Reader
	out bytes.Buffer
}

func (p *rwPair) Read(b []byte) (int, error)  { return p.in.Read(b) }
func (p *rwPair) Write(b []byte) (int, error) { return p.out.Write(b) }

// keyed is a connected Conn with application traffic keys installed, and the
// server's half conns for the same keys.
type keyed struct {
	c       *Conn
	rw      *rwPair
	srvOut  tlsraw.HalfConn
	srvIn   tlsraw.HalfConn
	records bytes.Buffer
}

func newKeyed(t testing.TB) *keyed {
	t.Helper()
	k := &keyed{c: new(Conn), rw: &rwPair{}}
	c := k.c
	if err := c.resetKeySchedule(); err != nil {
		t.Fatal(err)
	}
	serverSecret := bytes.Repeat([]byte{1}, hashSize)
	clientSecret := bytes.Repeat([]byte{2}, hashSize)
	mustInstall := func(hc *tlsraw.HalfConn, aead *aesgcm.Cipher, secret []byte) {
		if err := c.ks.InstallKeys(hc, aead, keyLen, secret); err != nil {
			t.Fatal(err)
		}
	}
	mustInstall(&c.in, &c.aeadIn, serverSecret)
	mustInstall(&c.out, &c.aeadOut, clientSecret)
	mustInstall(&k.srvOut, new(aesgcm.Cipher), serverSecret)
	mustInstall(&k.srvIn, new(aesgcm.Cipher), clientSecret)
	c.rw = k.rw
	c.connected = true
	return k
}

// send queues an encrypted server record of type ct.
func (k *keyed) send(t testing.TB, ct tlsraw.ContentType, content []byte) {
	t.Helper()
	var e tlsraw.Encoder
	buf := make([]byte, tlsraw.MaxRecord)
	e.Reset(buf, 0)
	rec := e.StartRecord(ct)
	e.Bytes(content)
	e.SealRecord(&k.srvOut, rec, ct)
	if e.Err() != nil {
		t.Fatal(e.Err())
	}
	k.records.Write(buf[:e.Len()])
}

func (k *keyed) start() { k.rw.in = bytes.NewReader(k.records.Bytes()) }

// clientRecords decrypts everything the client sent.
func (k *keyed) clientRecords(t *testing.T) (cts []tlsraw.ContentType, contents [][]byte) {
	t.Helper()
	out := k.rw.out.Bytes()
	for len(out) > 0 {
		n := tlsraw.SizeHeaderRecord + int(binary.BigEndian.Uint16(out[3:5]))
		content, ct, err := k.srvIn.Open(out[:n])
		if err != nil {
			t.Fatal(err)
		}
		cts = append(cts, ct)
		contents = append(contents, append([]byte(nil), content...))
		out = out[n:]
	}
	return cts, contents
}

func handshakeMsg(typ tlsraw.HandshakeType, body []byte) []byte {
	msg := []byte{byte(typ), byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	return append(msg, body...)
}

var (
	alertCloseNotify = []byte{byte(tlsraw.AlertLevelWarning), byte(tlsraw.AlertCloseNotify)}
	sessionTicket    = handshakeMsg(tlsraw.HandshakeTypeNewSessionTicket, bytes.Repeat([]byte{0x5a}, 300))
)

func readAll(t *testing.T, c *Conn) ([]byte, error) {
	t.Helper()
	var got []byte
	buf := make([]byte, 100)
	for {
		n, err := c.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			return got, err
		}
	}
}

func TestNewSessionTicketSpansRecords(t *testing.T) {
	k := newKeyed(t)
	k.send(t, tlsraw.ContentTypeHandshake, sessionTicket[:2]) // Even the header spans records.
	k.send(t, tlsraw.ContentTypeHandshake, sessionTicket[2:7])
	k.send(t, tlsraw.ContentTypeHandshake, sessionTicket[7:200])
	// The rest of the ticket shares a record with a whole second ticket.
	k.send(t, tlsraw.ContentTypeHandshake, append(append([]byte(nil), sessionTicket[200:]...), sessionTicket...))
	k.send(t, tlsraw.ContentTypeApplicationData, []byte("hello"))
	k.send(t, tlsraw.ContentTypeApplicationData, nil) // Zero-length application data is legal.
	k.send(t, tlsraw.ContentTypeApplicationData, []byte(" world"))
	k.send(t, tlsraw.ContentTypeAlert, alertCloseNotify)
	k.start()
	got, err := readAll(t, k.c)
	if err != io.EOF || string(got) != "hello world" {
		t.Fatalf("got %q, %v; want hello world, EOF", got, err)
	}
}

func TestRejectsInterleavedHandshakeFragment(t *testing.T) {
	for _, split := range []int{2, 100} { // Within the header and within the body.
		k := newKeyed(t)
		k.send(t, tlsraw.ContentTypeHandshake, sessionTicket[:split])
		k.send(t, tlsraw.ContentTypeApplicationData, []byte("data"))
		k.start()
		_, err := readAll(t, k.c)
		if err != AlertError(tlsraw.AlertUnexpectedMessage) {
			t.Fatalf("split %d: got %v, want unexpected_message", split, err)
		}
		wantAlertSent(t, k, tlsraw.AlertUnexpectedMessage)
	}
}

func TestPostHandshakeErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ct      tlsraw.ContentType
		content []byte
		want    error
	}{
		{"key update", tlsraw.ContentTypeHandshake, handshakeMsg(tlsraw.HandshakeTypeKeyUpdate, []byte{0}), ErrUnsupportedUpdate},
		{"finished", tlsraw.ContentTypeHandshake, handshakeMsg(tlsraw.HandshakeTypeFinished, make([]byte, 32)), AlertError(tlsraw.AlertUnexpectedMessage)},
		{"oversized ticket", tlsraw.ContentTypeHandshake, handshakeMsg(tlsraw.HandshakeTypeNewSessionTicket, make([]byte, HandshakeBufSize)), ErrMessageTooLarge},
		{"empty handshake record", tlsraw.ContentTypeHandshake, nil, AlertError(tlsraw.AlertUnexpectedMessage)},
		{"fatal alert", tlsraw.ContentTypeAlert, []byte{2, byte(tlsraw.AlertInternalError)}, PeerAlertError(tlsraw.AlertInternalError)},
		{"short alert", tlsraw.ContentTypeAlert, []byte{2}, AlertError(tlsraw.AlertDecodeError)},
		{"inner change_cipher_spec", tlsraw.ContentTypeChangeCipherSpec, []byte{1}, AlertError(tlsraw.AlertUnexpectedMessage)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newKeyed(t)
			k.send(t, tc.ct, tc.content)
			k.start()
			_, err := readAll(t, k.c)
			if err != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if _, err = k.c.Read(make([]byte, 1)); err != ErrClosed {
				t.Fatalf("read after failure: got %v, want ErrClosed", err)
			}
		})
	}
}

func TestRecordLayerErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mangle func([]byte) []byte
		want   error
	}{
		{"bad mac", func(r []byte) []byte { r[len(r)-1] ^= 1; return r }, AlertError(tlsraw.AlertBadRecordMAC)},
		{"plaintext handshake after keys", func(r []byte) []byte { r[0] = byte(tlsraw.ContentTypeHandshake); return r }, AlertError(tlsraw.AlertUnexpectedMessage)},
		{"overflow", func(r []byte) []byte {
			binary.BigEndian.PutUint16(r[3:5], tlsraw.MaxCiphertext+1)
			return r
		}, AlertError(tlsraw.AlertRecordOverflow)},
		{"truncated", func(r []byte) []byte { return r[:len(r)-3] }, io.ErrUnexpectedEOF},
		{"truncated header", func(r []byte) []byte { return r[:3] }, io.ErrUnexpectedEOF},
		{"eof without close_notify", func([]byte) []byte { return nil }, io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newKeyed(t)
			k.send(t, tlsraw.ContentTypeApplicationData, []byte("data"))
			k.rw.in = bytes.NewReader(tc.mangle(k.records.Bytes()))
			_, err := readAll(t, k.c)
			if err != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestWriteAndCloseNotify(t *testing.T) {
	k := newKeyed(t)
	k.start()
	big := bytes.Repeat([]byte{0xa5}, 1200) // Split into three records.
	if n, err := k.c.Write(big); err != nil || n != len(big) {
		t.Fatalf("write: %d, %v", n, err)
	}
	if err := k.c.Close(); err != nil {
		t.Fatal(err)
	}
	cts, contents := k.clientRecords(t)
	var data []byte
	for i, ct := range cts[:len(cts)-1] {
		if ct != tlsraw.ContentTypeApplicationData {
			t.Fatalf("record %d: %v", i, ct)
		}
		data = append(data, contents[i]...)
	}
	if len(cts) != 4 || !bytes.Equal(data, big) {
		t.Fatalf("got %d records, %d bytes", len(cts), len(data))
	}
	if last := contents[len(contents)-1]; cts[len(cts)-1] != tlsraw.ContentTypeAlert || !bytes.Equal(last, alertCloseNotify) {
		t.Fatalf("last record %v %x, want close_notify", cts[len(cts)-1], last)
	}
	if _, err := k.c.Write([]byte("x")); err != ErrClosed {
		t.Fatalf("write after close: %v", err)
	}
	if k.c.in.HasKeys() || k.c.out.HasKeys() {
		t.Fatal("keys survive Close")
	}
}

func wantAlertSent(t *testing.T, k *keyed, want tlsraw.AlertDescription) {
	t.Helper()
	cts, contents := k.clientRecords(t)
	if len(cts) != 1 || cts[0] != tlsraw.ContentTypeAlert || !bytes.Equal(contents[0], []byte{2, byte(want)}) {
		t.Fatalf("client sent %v %x, want fatal alert %d", cts, contents, want)
	}
}

// scripted answers the ClientHello with a server flight made from it.
type scripted struct {
	respond func(t *testing.T, clientHello []byte) []byte
	t       *testing.T
	in      *bytes.Reader
	out     bytes.Buffer
}

func (s *scripted) Read(b []byte) (int, error) {
	if s.in == nil {
		s.in = bytes.NewReader(s.respond(s.t, s.out.Bytes()))
	}
	return s.in.Read(b)
}

func (s *scripted) Write(b []byte) (int, error) { return s.out.Write(b) }

func plainRecord(ct tlsraw.ContentType, content []byte) []byte {
	rec := []byte{byte(ct), 3, 3, byte(len(content) >> 8), byte(len(content))}
	return append(rec, content...)
}

// serverHello builds a ServerHello answering clientHello, a whole record.
type serverHello struct {
	random  [32]byte
	suite   tlsraw.CipherSuite
	version uint16
	group   tlsraw.NamedGroup
	extra   []byte // Raw extensions appended after key_share.
	noShare bool
}

func (sh serverHello) record(t *testing.T, clientHello []byte) []byte {
	t.Helper()
	const sidOff = tlsraw.SizeHeaderRecord + tlsraw.SizeHeaderHandshake + 2 + 32
	sid := clientHello[sidOff+1 : sidOff+1+int(clientHello[sidOff])]
	var share [x25519.ShareSize]byte
	if _, err := new(x25519.Exchanger).ClientGenerateRekey(share[:], &countingRand{n: 100}); err != nil {
		t.Fatal(err)
	}
	var exts []byte
	exts = binary.BigEndian.AppendUint16(exts, uint16(tlsraw.ExtSupportedVersions))
	exts = binary.BigEndian.AppendUint16(exts, 2)
	exts = binary.BigEndian.AppendUint16(exts, sh.version)
	if !sh.noShare {
		exts = binary.BigEndian.AppendUint16(exts, uint16(tlsraw.ExtKeyShare))
		exts = binary.BigEndian.AppendUint16(exts, 4+uint16(len(share)))
		exts = binary.BigEndian.AppendUint16(exts, uint16(sh.group))
		exts = binary.BigEndian.AppendUint16(exts, uint16(len(share)))
		exts = append(exts, share[:]...)
	}
	exts = append(exts, sh.extra...)
	body := binary.BigEndian.AppendUint16(nil, tlsraw.VersionTLS12)
	body = append(body, sh.random[:]...)
	body = append(body, byte(len(sid)))
	body = append(body, sid...)
	body = binary.BigEndian.AppendUint16(body, uint16(sh.suite))
	body = append(body, 0)
	body = binary.BigEndian.AppendUint16(body, uint16(len(exts)))
	body = append(body, exts...)
	return plainRecord(tlsraw.ContentTypeHandshake, handshakeMsg(tlsraw.HandshakeTypeServerHello, body))
}

func goodHello() serverHello {
	return serverHello{random: [32]byte{1}, suite: tlsraw.SuiteAES128GCMSHA256, version: tlsraw.VersionTLS13, group: tlsraw.GroupX25519}
}

func TestHandshakeStateMachineErrors(t *testing.T) {
	encExt := plainRecord(tlsraw.ContentTypeHandshake, handshakeMsg(tlsraw.HandshakeTypeEncryptedExtensions, []byte{0, 0}))
	hrr := goodHello()
	hrr.random = helloRetryRandom
	wrongSuite := goodHello()
	wrongSuite.suite = 0x1302
	tls12 := goodHello()
	tls12.version = tlsraw.VersionTLS12
	p256 := goodHello()
	p256.group = 0x0017
	noShare := goodHello()
	noShare.noShare = true
	unsolicited := goodHello()
	unsolicited.extra = []byte{0, byte(tlsraw.ExtServerName), 0, 0}
	for _, tc := range []struct {
		name   string
		flight func(t *testing.T, ch []byte) []byte
		want   error
		alert  tlsraw.AlertDescription // Sent by the client; 0 for none.
	}{
		{"alert instead of hello", func(*testing.T, []byte) []byte {
			return plainRecord(tlsraw.ContentTypeAlert, []byte{2, byte(tlsraw.AlertHandshakeFailure)})
		}, PeerAlertError(tlsraw.AlertHandshakeFailure), 0},
		{"close before hello", func(*testing.T, []byte) []byte { return nil }, io.ErrUnexpectedEOF, 0},
		{"wrong first message", func(*testing.T, []byte) []byte { return encExt }, AlertError(tlsraw.AlertUnexpectedMessage), tlsraw.AlertUnexpectedMessage},
		{"application data before keys", func(*testing.T, []byte) []byte {
			return plainRecord(tlsraw.ContentTypeApplicationData, []byte("x"))
		}, AlertError(tlsraw.AlertUnexpectedMessage), tlsraw.AlertUnexpectedMessage},
		{"plaintext overflow", func(*testing.T, []byte) []byte {
			return plainRecord(tlsraw.ContentTypeHandshake, make([]byte, tlsraw.MaxPlaintext+1))
		}, AlertError(tlsraw.AlertRecordOverflow), tlsraw.AlertRecordOverflow},
		{"hello retry", hrr.record, ErrHelloRetry, tlsraw.AlertHandshakeFailure},
		{"wrong suite", wrongSuite.record, AlertError(tlsraw.AlertIllegalParameter), tlsraw.AlertIllegalParameter},
		{"tls 1.2", tls12.record, AlertError(tlsraw.AlertProtocolVersion), tlsraw.AlertProtocolVersion},
		{"p-256 share", p256.record, AlertError(tlsraw.AlertIllegalParameter), tlsraw.AlertIllegalParameter},
		{"no key share", noShare.record, AlertError(tlsraw.AlertMissingExtension), tlsraw.AlertMissingExtension},
		{"unsolicited extension", unsolicited.record, AlertError(tlsraw.AlertUnsupportedExtension), tlsraw.AlertUnsupportedExtension},
		{"hello and plaintext extensions", func(t *testing.T, ch []byte) []byte {
			// EncryptedExtensions must come encrypted: buffered past the key change.
			rec := goodHello().record(t, ch)
			msg := handshakeMsg(tlsraw.HandshakeTypeEncryptedExtensions, []byte{0, 0})
			rec = append(rec[:tlsraw.SizeHeaderRecord:tlsraw.SizeHeaderRecord], append(rec[tlsraw.SizeHeaderRecord:], msg...)...)
			binary.BigEndian.PutUint16(rec[3:5], uint16(len(rec)-tlsraw.SizeHeaderRecord))
			return rec
		}, AlertError(tlsraw.AlertUnexpectedMessage), tlsraw.AlertUnexpectedMessage},
		{"plaintext after hello", func(t *testing.T, ch []byte) []byte {
			return append(goodHello().record(t, ch), encExt...)
		}, AlertError(tlsraw.AlertUnexpectedMessage), tlsraw.AlertUnexpectedMessage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &scripted{respond: tc.flight, t: t}
			var c Conn
			err := c.Handshake(s, Config{ServerName: "tiny.test", Rand: &countingRand{}, InsecureSkipVerify: true})
			if err != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			// The ClientHello is the first record; any alert follows it.
			out := s.out.Bytes()
			out = out[tlsraw.SizeHeaderRecord+int(binary.BigEndian.Uint16(out[3:5])):]
			if tc.alert == 0 {
				if len(out) != 0 {
					t.Fatalf("client sent %x after the hello, want nothing", out)
				}
				return
			}
			if len(out) == 0 {
				t.Fatalf("client sent no alert, want %d", tc.alert)
			}
			// Before keys the alert is plaintext; after ServerHello it is encrypted
			// and only its presence is checked.
			if out[0] == byte(tlsraw.ContentTypeAlert) && !bytes.Equal(out[tlsraw.SizeHeaderRecord:], []byte{2, byte(tc.alert)}) {
				t.Fatalf("client sent alert %x, want %d", out[tlsraw.SizeHeaderRecord:], tc.alert)
			}
		})
	}
}

func TestConfigErrors(t *testing.T) {
	var v nopVerifier
	long := string(bytes.Repeat([]byte{'a'}, maxServerName+1))
	for _, tc := range []struct {
		name string
		cfg  Config
		want error
	}{
		{"no rand", Config{ServerName: "a.test", Verifier: v}, ErrNoRand},
		{"no verifier", Config{ServerName: "a.test", Rand: &countingRand{}}, ErrInsecure},
		{"no name", Config{Rand: &countingRand{}, Verifier: v}, ErrNoServerName},
		{"long name", Config{ServerName: long, Rand: &countingRand{}, Verifier: v}, ErrServerName},
		{"short handshake buffer", Config{ServerName: "a.test", Rand: &countingRand{}, Verifier: v, HandshakeBuf: make([]byte, HandshakeBufSize-1)}, ErrShortHandshakeBuf},
	} {
		var c Conn
		if err := c.Handshake(nil, tc.cfg); err != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

type nopVerifier struct{}

func (nopVerifier) VerifyPeer(lcrypto.CertChain, uint16, bool, []byte, []byte, []byte) error {
	return errors.New("unused")
}

func TestSNIOmitsIPLiterals(t *testing.T) {
	for _, tc := range []struct {
		name string
		sni  bool
	}{{"tiny-dash.apps.example.net", true}, {"192.168.1.10", false}, {"fe80::1", false}, {"1.example", true}, {"localhost", true}} {
		s := &scripted{respond: func(*testing.T, []byte) []byte { return nil }, t: t}
		var c Conn
		c.Handshake(s, Config{ServerName: tc.name, Rand: &countingRand{}, InsecureSkipVerify: true})
		if got := bytes.Contains(s.out.Bytes(), []byte(tc.name)); got != tc.sni {
			t.Errorf("%s: SNI sent %v, want %v", tc.name, got, tc.sni)
		}
	}
}

func TestOfferedForCertificateVerify(t *testing.T) {
	for _, tc := range []struct {
		s    tlsraw.SignatureScheme
		want bool
	}{
		{tlsraw.SigRSAPSSRSAESHA256, true},
		{tlsraw.SigECDSAP256SHA256, true},
		{tlsraw.SigEd25519, true},
		{tlsraw.SigRSAPKCS1SHA256, false},  // Offered for certificates only.
		{0x0201, false},                    // rsa_pkcs1_sha1
		{0x0203, false},                    // ecdsa_sha1
		{tlsraw.SigECDSAP521SHA512, false}, // Not offered.
	} {
		if got := offeredForCertificateVerify(uint16(tc.s)); got != tc.want {
			t.Errorf("%#04x: got %v, want %v", uint16(tc.s), got, tc.want)
		}
	}
}

func TestCertChainParse(t *testing.T) {
	entry := func(cert []byte, ext []byte) []byte {
		b := []byte{byte(len(cert) >> 16), byte(len(cert) >> 8), byte(len(cert))}
		b = append(b, cert...)
		b = binary.BigEndian.AppendUint16(b, uint16(len(ext)))
		return append(b, ext...)
	}
	body := func(ctx byte, entries ...[]byte) []byte {
		var list []byte
		for _, e := range entries {
			list = append(list, e...)
		}
		b := []byte{ctx, byte(len(list) >> 16), byte(len(list) >> 8), byte(len(list))}
		return append(b, list...)
	}
	var cc certChain
	if err := cc.parse(body(0, entry([]byte("leaf"), []byte{0, 5, 0, 0}), entry([]byte("ca"), nil))); err != nil {
		t.Fatal(err)
	}
	if leaf, _ := cc.CertView(0); cc.NumCerts() != 2 || string(leaf) != "leaf" {
		t.Fatalf("got %d certs, leaf %q", cc.NumCerts(), leaf)
	}
	if _, err := cc.CertView(2); err == nil {
		t.Fatal("CertView past the end succeeded")
	}
	var nine [][]byte
	for range maxPeerCerts + 1 {
		nine = append(nine, entry([]byte("c"), nil))
	}
	for name, b := range map[string][]byte{
		"empty":            nil,
		"context":          body(1, entry([]byte("leaf"), nil)),
		"no certs":         body(0),
		"empty cert":       body(0, entry(nil, nil)),
		"list too long":    body(0, entry([]byte("leaf"), nil))[:8],
		"ext overrun":      body(0, append(entry([]byte("leaf"), nil)[:7], 0, 9)),
		"too many certs":   body(0, nine...),
		"trailing garbage": append(body(0, entry([]byte("leaf"), nil)), 0),
	} {
		if err := cc.parse(b); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}
