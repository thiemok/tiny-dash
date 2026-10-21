// Package tinytls is a heapless TLS 1.3 client for microcontrollers. It runs the
// client handshake of RFC 8446 on top of lneto's tlsraw record layer and key
// schedule and lcrypto's primitives, over any reliable byte stream.
//
// It implements exactly one suite (TLS_AES_128_GCM_SHA256) and one group
// (X25519), no PSK or resumption, no HelloRetryRequest, no client certificates
// and no KeyUpdate. A Conn is about 21 KiB, plus an 8 KiB handshake buffer that
// it only needs during [Conn.Handshake] and can borrow from the caller, see
// [Config.HandshakeBuf]. It allocates nothing after its first handshake, so
// create it once and reuse it with [Conn.Handshake] for every connection.
package tinytls

import (
	"encoding/binary"
	"io"

	"github.com/soypat/lcrypto"
	"github.com/soypat/lcrypto/aesgcm"
	"github.com/soypat/lcrypto/sha256"
	"github.com/soypat/lcrypto/x25519"
	"github.com/soypat/lneto"
	"github.com/soypat/lneto/crypto/tlsraw"
)

const (
	keyLen   = 16
	hashSize = sha256.Size
	// sizeWrite holds the largest ClientHello, 427 B with a 253 B ServerName.
	// Larger writes are split across records.
	sizeWrite     = 512
	maxPeerCerts  = 8
	maxServerName = 253
	cvContext     = "TLS 1.3, server CertificateVerify"
	sizeCVMsg     = 64 + len(cvContext) + 1 + hashSize
)

// HandshakeBufSize is the size of [Config.HandshakeBuf]. It holds one handshake
// message: the server's Certificate is the largest, ~4.5 KB for a Let's Encrypt
// RSA chain.
const HandshakeBufSize = 8192

var helloRetryRandom = [32]byte{
	0xcf, 0x21, 0xad, 0x74, 0xe5, 0x9a, 0x61, 0x11, 0xbe, 0x1d, 0x8c, 0x02, 0x1e, 0x65, 0xb8, 0x91,
	0xc2, 0xa2, 0x11, 0x16, 0x7a, 0xbb, 0x8c, 0x5e, 0x07, 0x9e, 0x09, 0xe2, 0xc8, 0xa8, 0x33, 0x9c,
}

// offeredSchemes are sent in signature_algorithms. The PKCS #1 v1.5 schemes
// only cover signatures in certificates, RFC 8446 4.2.3: a CertificateVerify
// must use one of the others.
var offeredSchemes = [...]tlsraw.SignatureScheme{
	tlsraw.SigECDSAP256SHA256,
	tlsraw.SigRSAPSSRSAESHA256,
	tlsraw.SigECDSAP384SHA384,
	tlsraw.SigRSAPSSRSAESHA384,
	tlsraw.SigRSAPSSRSAESHA512,
	tlsraw.SigEd25519,
	tlsraw.SigRSAPKCS1SHA256,
	tlsraw.SigRSAPKCS1SHA384,
	tlsraw.SigRSAPKCS1SHA512,
}

// Config configures a [Conn].
type Config struct {
	// ServerName is sent as SNI, unless it is an IP literal, and is the name the
	// Verifier checks the leaf certificate against. It is required with a
	// Verifier.
	ServerName string
	// Rand provides the hello random, session ID and key share entropy. It must
	// be a cryptographically secure source.
	Rand io.Reader
	// Verifier authenticates the server's chain and CertificateVerify
	// signature, e.g. a configured *x509.Verifier of lcrypto.
	Verifier lcrypto.Verifier
	// InsecureSkipVerify accepts any server when Verifier is nil. The traffic is
	// encrypted but an active attacker can impersonate the server. It exists for
	// tests and measurements only.
	InsecureSkipVerify bool
	// HandshakeBuf, if not nil, is borrowed as scratch during Handshake and
	// must hold at least HandshakeBufSize bytes; the Conn does not touch it
	// once Handshake returns. If nil, the Conn allocates its own on the first
	// Handshake and keeps it.
	HandshakeBuf []byte
}

// Conn is a TLS 1.3 client connection. It is not safe for concurrent use.
type Conn struct {
	rw  io.ReadWriter
	cfg Config

	ks              tlsraw.KeySchedule
	ksConfigured    bool
	in, out         tlsraw.HalfConn
	aeadIn, aeadOut aesgcm.Cipher
	transcript, mac sha256.Digest
	exch            x25519.Exchanger
	vld             lneto.Validator
	chain           certChain

	connected  bool
	peerClosed bool
	app        []byte // Received application data not yet returned by Read, points into rbuf.

	hbuf         []byte // Handshake messages, during Handshake only.
	ownHbuf      []byte // hbuf when Config.HandshakeBuf is nil.
	hsOff, hsEnd int    // hbuf[hsOff:hsEnd] is handshake data not yet handled.
	hsKeep       int    // hbuf[:hsKeep] is retained across record reads (the Certificate message).

	// A post-handshake message is skipped as it arrives instead of buffered:
	// postHdr[:postHdrLen] is its header so far, then postSkip body bytes follow.
	postHdr    [tlsraw.SizeHeaderHandshake]byte
	postHdrLen int
	postSkip   int

	clientHS, serverHS [hashSize]byte
	clientAP, serverAP [hashSize]byte
	finished           [hashSize]byte
	shared             [x25519.SharedSize]byte
	sessionID          [32]byte
	cvMsg              [sizeCVMsg]byte
	name               [maxServerName]byte // ServerName as bytes: converting a string per handshake would allocate.
	nameLen            int

	rbuf [tlsraw.MaxRecord]byte
	wbuf [sizeWrite]byte
}

// Handshake resets c and runs the client handshake over rw. rw must be a
// blocking stream; its deadlines bound the handshake. On error c is wiped and
// an alert has been sent where one is due; the caller still owns rw.
func (c *Conn) Handshake(rw io.ReadWriter, cfg Config) error {
	switch {
	case cfg.Rand == nil:
		return ErrNoRand
	case cfg.Verifier == nil && !cfg.InsecureSkipVerify:
		return ErrInsecure
	case cfg.Verifier != nil && cfg.ServerName == "":
		return ErrNoServerName
	case len(cfg.ServerName) > maxServerName:
		return ErrServerName
	case cfg.HandshakeBuf != nil && len(cfg.HandshakeBuf) < HandshakeBufSize:
		return ErrShortHandshakeBuf
	}
	c.reset()
	c.nameLen = copy(c.name[:], cfg.ServerName)
	c.rw = rw
	c.cfg = cfg
	c.cfg.HandshakeBuf = nil
	if cfg.HandshakeBuf != nil {
		c.hbuf = cfg.HandshakeBuf[:HandshakeBufSize]
	} else {
		if c.ownHbuf == nil {
			c.ownHbuf = make([]byte, HandshakeBufSize)
		}
		c.hbuf = c.ownHbuf
	}
	err := c.resetKeySchedule()
	if err == nil {
		err = c.handshake()
	}
	// Nothing after the handshake uses hbuf: post-handshake messages are
	// skipped as they arrive, and records and alerts live in rbuf and wbuf.
	c.hbuf = nil
	if err != nil {
		return c.fail(err)
	}
	c.connected = true
	return nil
}

// Read reads application data. It returns io.EOF after the server's close_notify.
func (c *Conn) Read(p []byte) (int, error) {
	if !c.connected {
		return 0, ErrClosed
	}
	for len(c.app) == 0 {
		if c.peerClosed {
			return 0, io.EOF
		}
		if err := c.readApp(); err != nil {
			return 0, c.fail(err)
		}
	}
	n := copy(p, c.app)
	c.app = c.app[n:]
	return n, nil
}

// Write sends p as application data records.
func (c *Conn) Write(p []byte) (n int, err error) {
	if !c.connected {
		return 0, ErrClosed
	}
	maxChunk := len(c.wbuf) - tlsraw.SizeHeaderRecord - c.out.Overhead()
	for len(p) > 0 {
		chunk := p[:min(len(p), maxChunk)]
		if err = c.writeRecord(tlsraw.ContentTypeApplicationData, chunk); err != nil {
			return n, c.fail(err)
		}
		n += len(chunk)
		p = p[len(chunk):]
	}
	return n, nil
}

// Close sends close_notify, if the connection is up, and wipes the connection
// secrets, leaving c ready for another Handshake. It does not wait for the
// server's close_notify, nor close the underlying stream.
func (c *Conn) Close() error {
	var err error
	if c.connected {
		err = c.writeAlert(tlsraw.AlertCloseNotify)
	}
	c.Zeroize()
	return err
}

// Zeroize wipes all secrets without sending close_notify.
func (c *Conn) Zeroize() {
	c.in.Zeroize()
	c.out.Zeroize()
	if c.ksConfigured {
		// FastReset, unlike Zeroize, wipes the schedule without allocating: it
		// overwrites the secret with the public early secret, and the paranoid
		// schedule cleared its scratch buffers after every use.
		c.ks.FastReset()
	}
	c.exch.Zeroize()
	clear(c.clientHS[:])
	clear(c.serverHS[:])
	clear(c.clientAP[:])
	clear(c.serverAP[:])
	clear(c.finished[:])
	clear(c.shared[:])
	clear(c.rbuf[:])
	c.connected = false
	c.app = nil
}

func (c *Conn) reset() {
	c.Zeroize()
	c.peerClosed = false
	c.hsOff, c.hsEnd, c.hsKeep = 0, 0, 0
	c.postHdrLen, c.postSkip = 0, 0
	c.chain = certChain{}
	c.vld.ResetErr()
}

// resetKeySchedule starts a fresh transcript at the early secret. Configure
// heap-allocates through KeySchedule.Zeroize (a 496 B composite literal), so
// it runs once per Conn; later handshakes reuse its hashes with FastReset.
func (c *Conn) resetKeySchedule() error {
	if c.ksConfigured {
		c.ks.FastReset()
		return nil
	}
	c.transcript.Init()
	c.mac.Init()
	if err := c.ks.Configure(&c.transcript, &c.mac, true); err != nil {
		return err
	}
	c.ksConfigured = true
	return nil
}

func (c *Conn) handshake() error {
	if err := c.writeClientHello(); err != nil {
		return err
	}
	msg, err := c.readHandshake(tlsraw.HandshakeTypeServerHello)
	if err != nil {
		return err
	}
	if err = c.handleServerHello(msg); err != nil {
		return err
	}
	c.ks.AddMessage(msg)
	c.ks.Handshake(c.clientHS[:], c.serverHS[:], c.shared[:])
	clear(c.shared[:])
	if err = c.keyChangeBoundary(); err != nil {
		return err
	}
	if err = c.ks.InstallKeys(&c.in, &c.aeadIn, keyLen, c.serverHS[:]); err != nil {
		return err
	}
	if err = c.ks.InstallKeys(&c.out, &c.aeadOut, keyLen, c.clientHS[:]); err != nil {
		return err
	}

	msg, err = c.readHandshake(tlsraw.HandshakeTypeEncryptedExtensions)
	if err != nil {
		return err
	}
	c.ks.AddMessage(msg)

	msg, err = c.readHandshake(tlsraw.HandshakeTypeCertificate)
	if err != nil {
		return err
	}
	if err = c.chain.parse(msg[tlsraw.SizeHeaderHandshake:]); err != nil {
		return err
	}
	c.hsKeep = c.hsOff // The chain points into hbuf until CertificateVerify is checked.
	c.ks.AddMessage(msg)

	msg, err = c.readHandshake(tlsraw.HandshakeTypeCertificateVerify)
	if err != nil {
		return err
	}
	if err = c.verifyServer(msg[tlsraw.SizeHeaderHandshake:]); err != nil {
		return err
	}
	c.hsKeep = 0
	c.ks.AddMessage(msg)

	msg, err = c.readHandshake(tlsraw.HandshakeTypeFinished)
	if err != nil {
		return err
	}
	c.ks.Finished(c.finished[:], c.serverHS[:])
	if verify := msg[tlsraw.SizeHeaderHandshake:]; !ctEqual(verify, c.finished[:]) {
		return AlertError(tlsraw.AlertDecryptError)
	}
	if err = c.keyChangeBoundary(); err != nil {
		return err
	}
	c.ks.AddMessage(msg)
	c.ks.Master(c.clientAP[:], c.serverAP[:])

	c.ks.Finished(c.finished[:], c.clientHS[:])
	if err = c.writeFinished(); err != nil {
		return err
	}
	c.in.Zeroize()
	c.out.Zeroize()
	if err = c.ks.InstallKeys(&c.in, &c.aeadIn, keyLen, c.serverAP[:]); err != nil {
		return err
	}
	if err = c.ks.InstallKeys(&c.out, &c.aeadOut, keyLen, c.clientAP[:]); err != nil {
		return err
	}
	clear(c.clientHS[:])
	clear(c.serverHS[:])
	clear(c.clientAP[:])
	clear(c.serverAP[:])
	c.ks.FastReset()
	c.hsOff, c.hsEnd = 0, 0
	return nil
}

func (c *Conn) writeClientHello() error {
	var e tlsraw.Encoder
	e.Reset(c.wbuf[:], 0)
	rec := e.StartRecord(tlsraw.ContentTypeHandshake)
	start := e.StartMessage(tlsraw.HandshakeTypeClientHello)
	e.Uint16(tlsraw.VersionTLS12)
	if err := c.random(e.Reserve(tlsraw.SizeHelloRandom)); err != nil {
		return err
	}
	if err := c.random(c.sessionID[:]); err != nil {
		return err
	}
	e.Uint8(uint8(len(c.sessionID)))
	e.Bytes(c.sessionID[:])
	e.Uint16(2)
	e.Uint16(uint16(tlsraw.SuiteAES128GCMSHA256))
	e.Uint8(1) // legacy_compression_methods: null only.
	e.Uint8(0)

	exts := e.Open(2)

	if name := c.cfg.ServerName; name != "" && !isIPLiteral(name) {
		e.Uint16(uint16(tlsraw.ExtServerName))
		ext := e.Open(2)
		list := e.Open(2)
		e.Uint8(0) // host_name
		e.Uint16(uint16(len(name)))
		copy(e.Reserve(len(name)), name)
		e.Close(list, 2)
		e.Close(ext, 2)
	}

	e.Uint16(uint16(tlsraw.ExtSupportedVersions))
	e.Uint16(3)
	e.Uint8(2)
	e.Uint16(tlsraw.VersionTLS13)

	e.Uint16(uint16(tlsraw.ExtSupportedGroups))
	e.Uint16(4)
	e.Uint16(2)
	e.Uint16(uint16(tlsraw.GroupX25519))

	e.Uint16(uint16(tlsraw.ExtSignatureAlgorithms))
	e.Uint16(uint16(2 + 2*len(offeredSchemes)))
	e.Uint16(uint16(2 * len(offeredSchemes)))
	for _, s := range offeredSchemes {
		e.Uint16(uint16(s))
	}

	e.Uint16(uint16(tlsraw.ExtKeyShare))
	e.Uint16(2 + 4 + x25519.ShareSize)
	e.Uint16(4 + x25519.ShareSize)
	e.Uint16(uint16(tlsraw.GroupX25519))
	e.Uint16(x25519.ShareSize)
	share := e.Reserve(x25519.ShareSize)
	if share != nil {
		if _, err := c.exch.ClientGenerateRekey(share, c.cfg.Rand); err != nil {
			return err
		}
	}

	e.Close(exts, 2)
	msg := e.EndMessage(start)
	e.EndRecord(rec)
	if e.Err() != nil {
		return e.Err()
	}
	c.ks.AddMessage(msg)
	return c.flush(&e)
}

func (c *Conn) random(dst []byte) error {
	if dst == nil {
		return lneto.ErrShortBuffer
	}
	_, err := io.ReadFull(c.cfg.Rand, dst)
	return err
}

func (c *Conn) handleServerHello(msg []byte) error {
	var sh tlsraw.HelloServerMsg
	if _, err := sh.Decode(msg[tlsraw.SizeHeaderHandshake:], &c.vld); err != nil {
		c.vld.ResetErr()
		return AlertError(tlsraw.AlertDecodeError)
	}
	if *sh.Random() == helloRetryRandom {
		return ErrHelloRetry
	}
	if sh.CipherSuite() != tlsraw.SuiteAES128GCMSHA256 || sh.Compression() != 0 ||
		!ctEqual(sh.SessionID(), c.sessionID[:]) {
		return AlertError(tlsraw.AlertIllegalParameter)
	}
	var version uint16
	var serverShare []byte
	for exts := sh.Extensions(); len(exts) > 0; {
		ext, err := tlsraw.NewExtensionFrame(exts)
		if err != nil {
			return AlertError(tlsraw.AlertDecodeError)
		}
		exts = exts[len(ext.RawData()):]
		ext.ValidateType(&c.vld, true)
		if c.vld.HasError() {
			c.vld.ResetErr()
			return AlertError(tlsraw.AlertDecodeError)
		}
		switch ext.Type() {
		case tlsraw.ExtSupportedVersions:
			if version != 0 || len(ext.Data()) != 2 {
				return AlertError(tlsraw.AlertDecodeError)
			}
			version = binary.BigEndian.Uint16(ext.Data())
		case tlsraw.ExtKeyShare:
			if serverShare != nil {
				return AlertError(tlsraw.AlertDecodeError)
			}
			group, key, n, err := tlsraw.NextKeyShare(ext.Data(), true)
			if err != nil || n != len(ext.Data()) {
				return AlertError(tlsraw.AlertDecodeError)
			} else if group != tlsraw.GroupX25519 {
				return AlertError(tlsraw.AlertIllegalParameter)
			}
			serverShare = key
		default:
			// RFC 8446 4.1.3: only extensions the client offered may appear, and
			// pre_shared_key was not offered.
			return AlertError(tlsraw.AlertUnsupportedExtension)
		}
	}
	if version != tlsraw.VersionTLS13 {
		return AlertError(tlsraw.AlertProtocolVersion)
	} else if serverShare == nil {
		return AlertError(tlsraw.AlertMissingExtension)
	}
	_, err := c.exch.ClientShared(c.shared[:], serverShare)
	c.exch.Zeroize()
	if err != nil {
		return AlertError(tlsraw.AlertIllegalParameter)
	}
	return nil
}

func (c *Conn) verifyServer(body []byte) error {
	if len(body) < 4 {
		return AlertError(tlsraw.AlertDecodeError)
	}
	scheme := binary.BigEndian.Uint16(body)
	sigLen := int(binary.BigEndian.Uint16(body[2:]))
	if sigLen != len(body)-4 {
		return AlertError(tlsraw.AlertDecodeError)
	} else if !offeredForCertificateVerify(scheme) {
		return AlertError(tlsraw.AlertIllegalParameter)
	}
	if c.cfg.Verifier == nil {
		return nil // InsecureSkipVerify.
	}
	for i := range 64 {
		c.cvMsg[i] = 0x20
	}
	copy(c.cvMsg[64:], cvContext)
	c.cvMsg[64+len(cvContext)] = 0
	c.ks.TranscriptHash(c.cvMsg[64+len(cvContext)+1:])
	err := c.cfg.Verifier.VerifyPeer(&c.chain, scheme, true, c.name[:c.nameLen], c.cvMsg[:], body[4:])
	if err != nil {
		return VerifyError{Err: err}
	}
	return nil
}

func (c *Conn) writeFinished() error {
	var e tlsraw.Encoder
	e.Reset(c.wbuf[:], 0)
	rec := e.StartRecord(tlsraw.ContentTypeHandshake)
	start := e.StartMessage(tlsraw.HandshakeTypeFinished)
	e.Bytes(c.finished[:])
	e.EndMessage(start)
	e.SealRecord(&c.out, rec, tlsraw.ContentTypeHandshake)
	clear(c.finished[:])
	return c.flush(&e)
}

// keyChangeBoundary rejects handshake data buffered past a key change, RFC 8446 5.1.
func (c *Conn) keyChangeBoundary() error {
	if c.hsOff != c.hsEnd {
		return AlertError(tlsraw.AlertUnexpectedMessage)
	}
	return nil
}

// readHandshake returns the next handshake message, header included, which must
// be of type want. The message points into hbuf.
func (c *Conn) readHandshake(want tlsraw.HandshakeType) ([]byte, error) {
	for {
		msg, err := c.nextHandshake()
		if err != nil {
			return nil, err
		} else if msg != nil {
			if tlsraw.HandshakeType(msg[0]) != want {
				return nil, AlertError(tlsraw.AlertUnexpectedMessage)
			}
			return msg, nil
		}
		ct, content, err := c.readRecord()
		if err != nil {
			return nil, err
		}
		switch ct {
		case tlsraw.ContentTypeHandshake:
			if err = c.addHandshake(content); err != nil {
				return nil, err
			}
		case tlsraw.ContentTypeAlert:
			if err = c.handleAlert(content); err != nil {
				return nil, err
			}
			return nil, io.ErrUnexpectedEOF // close_notify or user_canceled mid-handshake.
		default:
			return nil, AlertError(tlsraw.AlertUnexpectedMessage)
		}
	}
}

// nextHandshake returns the next complete buffered handshake message, header
// included, or nil if more records are needed.
func (c *Conn) nextHandshake() ([]byte, error) {
	pending := c.hbuf[c.hsOff:c.hsEnd]
	if len(pending) < tlsraw.SizeHeaderHandshake {
		return nil, nil
	}
	end := tlsraw.SizeHeaderHandshake + uint24(pending[1:])
	if end > HandshakeBufSize-c.hsKeep {
		return nil, ErrMessageTooLarge
	} else if end > len(pending) {
		return nil, nil
	}
	c.hsOff += end
	return pending[:end], nil
}

// addHandshake buffers a handshake record's content after the unhandled data,
// which it first moves down to hbuf[hsKeep:].
func (c *Conn) addHandshake(content []byte) error {
	if len(content) == 0 {
		return AlertError(tlsraw.AlertUnexpectedMessage) // RFC 8446 5.1: no zero-length handshake fragments.
	}
	c.hsEnd = c.hsKeep + copy(c.hbuf[c.hsKeep:], c.hbuf[c.hsOff:c.hsEnd])
	c.hsOff = c.hsKeep
	if len(content) > len(c.hbuf)-c.hsEnd {
		return ErrMessageTooLarge
	}
	c.hsEnd += copy(c.hbuf[c.hsEnd:], content)
	return nil
}

// readApp reads one record after the handshake, leaving application data in c.app.
func (c *Conn) readApp() error {
	ct, content, err := c.readRecord()
	if err != nil {
		return err
	}
	if ct != tlsraw.ContentTypeHandshake && (c.postHdrLen != 0 || c.postSkip != 0) {
		// RFC 8446 5.1: a message split across records must not be interleaved.
		return AlertError(tlsraw.AlertUnexpectedMessage)
	}
	switch ct {
	case tlsraw.ContentTypeApplicationData:
		c.app = content
		return nil
	case tlsraw.ContentTypeAlert:
		return c.handleAlert(content)
	case tlsraw.ContentTypeHandshake:
		return c.skipPostHandshake(content)
	}
	return AlertError(tlsraw.AlertUnexpectedMessage)
}

// skipPostHandshake drops NewSessionTicket messages, RFC 8446 4.6.1, which
// may span records; the rest of an incomplete one is skipped from the next.
func (c *Conn) skipPostHandshake(content []byte) error {
	if len(content) == 0 {
		return AlertError(tlsraw.AlertUnexpectedMessage) // RFC 8446 5.1: no zero-length handshake fragments.
	}
	for len(content) > 0 {
		if c.postSkip > 0 {
			n := min(c.postSkip, len(content))
			c.postSkip -= n
			content = content[n:]
			continue
		}
		n := copy(c.postHdr[c.postHdrLen:], content)
		c.postHdrLen += n
		content = content[n:]
		if c.postHdrLen < len(c.postHdr) {
			return nil
		}
		c.postHdrLen = 0
		switch tlsraw.HandshakeType(c.postHdr[0]) {
		case tlsraw.HandshakeTypeNewSessionTicket:
		case tlsraw.HandshakeTypeKeyUpdate:
			return ErrUnsupportedUpdate
		default:
			return AlertError(tlsraw.AlertUnexpectedMessage)
		}
		c.postSkip = uint24(c.postHdr[1:])
		if tlsraw.SizeHeaderHandshake+c.postSkip > HandshakeBufSize {
			return ErrMessageTooLarge // As during the handshake, which buffers messages.
		}
	}
	return nil
}

// readRecord reads one record and returns its content type and content,
// decrypted once keys are installed. content points into rbuf.
func (c *Conn) readRecord() (tlsraw.ContentType, []byte, error) {
	for {
		hdr := c.rbuf[:tlsraw.SizeHeaderRecord]
		if _, err := io.ReadFull(c.rw, hdr); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF // Only close_notify ends a connection cleanly.
			}
			return 0, nil, err
		}
		n := int(binary.BigEndian.Uint16(hdr[3:5]))
		if n > tlsraw.MaxCiphertext {
			return 0, nil, AlertError(tlsraw.AlertRecordOverflow)
		}
		rec := c.rbuf[:tlsraw.SizeHeaderRecord+n]
		if _, err := io.ReadFull(c.rw, rec[tlsraw.SizeHeaderRecord:]); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return 0, nil, err
		}
		ct := tlsraw.ContentType(rec[0])
		content := rec[tlsraw.SizeHeaderRecord:]
		switch {
		case ct == tlsraw.ContentTypeChangeCipherSpec:
			if c.connected || n != 1 || content[0] != 1 {
				return 0, nil, AlertError(tlsraw.AlertUnexpectedMessage)
			}
			continue // Middlebox compatibility, RFC 8446 D.4.
		case c.in.HasKeys():
			if ct != tlsraw.ContentTypeApplicationData {
				return 0, nil, AlertError(tlsraw.AlertUnexpectedMessage)
			}
			var err error
			content, ct, err = c.in.Open(rec)
			if err != nil {
				return 0, nil, AlertError(tlsraw.AlertBadRecordMAC)
			}
		case ct != tlsraw.ContentTypeHandshake && ct != tlsraw.ContentTypeAlert:
			return 0, nil, AlertError(tlsraw.AlertUnexpectedMessage)
		}
		if len(content) > tlsraw.MaxPlaintext {
			return 0, nil, AlertError(tlsraw.AlertRecordOverflow)
		}
		return ct, content, nil
	}
}

func (c *Conn) handleAlert(content []byte) error {
	if len(content) != 2 {
		return AlertError(tlsraw.AlertDecodeError)
	}
	switch desc := tlsraw.AlertDescription(content[1]); desc {
	case tlsraw.AlertCloseNotify:
		c.peerClosed = true
		return nil
	case tlsraw.AlertUserCanceled:
		return nil
	default:
		return PeerAlertError(desc)
	}
}

func (c *Conn) writeRecord(ct tlsraw.ContentType, content []byte) error {
	var e tlsraw.Encoder
	e.Reset(c.wbuf[:], 0)
	rec := e.StartRecord(ct)
	e.Bytes(content)
	if c.out.HasKeys() {
		e.SealRecord(&c.out, rec, ct)
	} else {
		e.EndRecord(rec)
	}
	return c.flush(&e)
}

func (c *Conn) writeAlert(desc tlsraw.AlertDescription) error {
	level := tlsraw.AlertLevelFatal
	if desc == tlsraw.AlertCloseNotify {
		level = tlsraw.AlertLevelWarning
	}
	alert := [2]byte{byte(level), byte(desc)}
	return c.writeRecord(tlsraw.ContentTypeAlert, alert[:])
}

func (c *Conn) flush(e *tlsraw.Encoder) error {
	if e.Err() != nil {
		return e.Err()
	}
	_, err := c.rw.Write(c.wbuf[:e.Len()])
	return err
}

// fail sends the alert for err, if any is due, and wipes the connection secrets.
// Sending is best effort: the connection is failing anyway.
func (c *Conn) fail(err error) error {
	switch a := err.(type) {
	case PeerAlertError:
	case AlertError:
		c.writeAlert(tlsraw.AlertDescription(a))
	case VerifyError:
		c.writeAlert(tlsraw.AlertBadCertificate)
	default:
		if err == ErrHelloRetry {
			c.writeAlert(tlsraw.AlertHandshakeFailure)
		} else if err != io.ErrUnexpectedEOF {
			c.writeAlert(tlsraw.AlertInternalError)
		}
	}
	c.Zeroize()
	return err
}

// offeredForCertificateVerify reports whether s is one of offeredSchemes and
// allowed in a CertificateVerify: TLS 1.3 forbids SHA-1 and RSASSA-PKCS1-v1_5
// there, RFC 8446 4.2.3.
func offeredForCertificateVerify(s uint16) bool {
	if hash, sig := s>>8, s&0xff; hash == 0x02 || (sig == 0x01 && hash <= 0x06) {
		return false
	}
	for _, o := range offeredSchemes {
		if uint16(o) == s {
			return true
		}
	}
	return false
}

// isIPLiteral reports whether name is an IPv4 or IPv6 literal, which RFC 6066
// 3 forbids in SNI. A DNS name never consists of digits and dots only, since
// top-level domains are not all-numeric, and never contains a colon.
func isIPLiteral(name string) bool {
	digitsAndDots := true
	for i := 0; i < len(name); i++ {
		switch b := name[i]; {
		case b == ':':
			return true
		case b != '.' && (b < '0' || b > '9'):
			digitsAndDots = false
		}
	}
	return digitsAndDots
}

func ctEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func uint24(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }
