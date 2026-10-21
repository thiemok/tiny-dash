//go:build !tinygo

package tinytls

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"testing"

	"github.com/soypat/lneto/crypto/tlsraw"
)

// FuzzServerFlight feeds mutated server flights, seeded with recorded real
// ones, to a validating client. It must fail cleanly, never panic.
func FuzzServerFlight(f *testing.F) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.Fatal(err)
	}
	ecFlight, ecVerifier := recordFlight(f, ecKey(f), 1000)
	rsaFlight, rsaVerifier := recordFlight(f, rsaKey, 1000)
	f.Add(ecFlight, false)
	f.Add(rsaFlight, true)
	var c Conn
	var p replay
	buf := make([]byte, 4096)
	f.Fuzz(func(t *testing.T, flight []byte, useRSA bool) {
		v := ecVerifier
		if useRSA {
			v = rsaVerifier
		}
		p.r.Reset(flight)
		if err := c.Handshake(&p, Config{ServerName: testName, Rand: &countingRand{}, Verifier: v}); err != nil {
			return
		}
		for {
			if _, err := c.Read(buf); err != nil {
				break
			}
		}
		c.Close()
	})
}

// FuzzPostHandshake feeds arbitrary handshake records to a connected client.
func FuzzPostHandshake(f *testing.F) {
	f.Add(sessionTicket[:10], sessionTicket[10:])
	f.Add(handshakeMsg(tlsraw.HandshakeTypeKeyUpdate, []byte{0}), []byte(nil))
	f.Add([]byte{4, 0xff, 0xff, 0xff}, []byte{0})
	f.Fuzz(func(t *testing.T, a, b []byte) {
		k := newKeyed(t)
		k.send(t, tlsraw.ContentTypeHandshake, a)
		k.send(t, tlsraw.ContentTypeHandshake, b)
		k.send(t, tlsraw.ContentTypeApplicationData, []byte("x"))
		k.start()
		_, err := io.ReadAll(k.c)
		if err == nil {
			t.Fatal("stream without close_notify ended cleanly")
		}
	})
}

func FuzzCertChainParse(f *testing.F) {
	f.Add([]byte{0, 0, 0, 9, 0, 0, 4, 'l', 'e', 'a', 'f', 0, 0})
	f.Fuzz(func(t *testing.T, body []byte) {
		var cc certChain
		if cc.parse(body) != nil {
			return
		}
		for i := range cc.NumCerts() {
			if der, err := cc.CertView(i); err != nil || len(der) == 0 {
				t.Fatalf("cert %d: %d bytes, %v", i, len(der), err)
			}
		}
	})
}
