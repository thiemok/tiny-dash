// Command tinygocheck links tinytls and its roots so `nx run tinytls:build:tinygo`
// proves they compile for the Pico target with TinyGo.
package main

import (
	"io"
	"time"

	"github.com/soypat/lcrypto/x509"
	"github.com/thiemok/tiny-dash/tinytls"
	"github.com/thiemok/tiny-dash/tinytls/drbg"
	"github.com/thiemok/tiny-dash/tinytls/roots"
)

var (
	chain    x509.Chain
	verifier x509.Verifier
	conn     tinytls.Conn
	rng      drbg.Reader
)

type closedStream struct{}

func (closedStream) Read([]byte) (int, error)    { return 0, io.EOF }
func (closedStream) Write(b []byte) (int, error) { return len(b), nil }

func main() {
	rng.Seed(make([]byte, drbg.MinSeed))
	chain = roots.ISRG()
	err := verifier.Configure(x509.VerifierConfig{Roots: &chain, Nanotime: func() int64 { return time.Now().UnixNano() }})
	if err != nil {
		panic(err)
	}
	err = conn.Handshake(closedStream{}, tinytls.Config{ServerName: "example.net", Rand: &rng, Verifier: &verifier})
	println(err.Error())
}
