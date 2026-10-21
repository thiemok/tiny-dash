package roots

import (
	"testing"
	"time"

	"github.com/soypat/lcrypto/x509"
)

func TestISRGConfiguresVerifier(t *testing.T) {
	chain := ISRG()
	var v x509.Verifier
	err := v.Configure(x509.VerifierConfig{Roots: &chain, Nanotime: func() int64 { return time.Now().UnixNano() }})
	if err != nil {
		t.Fatal(err)
	}
}

func TestISRGNotExpiring(t *testing.T) {
	soon := time.Now().AddDate(5, 0, 0)
	for i, der := range ISRG() {
		var c x509.Certificate
		if err := c.Parse(der); err != nil {
			t.Fatalf("root %d: %v", i, err)
		}
		if notAfter := time.Unix(c.NotAfter, 0); notAfter.Before(soon) {
			t.Errorf("root %d expires %v, within five years", i, notAfter)
		}
	}
}
