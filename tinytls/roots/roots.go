// Package roots holds trust anchors for tinytls as read-only DER kept in flash.
package roots

import (
	_ "embed"
	"unsafe"

	"github.com/soypat/lcrypto/x509"
)

// Strings are not copied into RAM by TinyGo, unlike []byte variables.
var (
	// SHA-256 96bcec06264976f37460779acf28c5a7cfe8a3c0aae11a8ffcee05c0bddf08c6.
	//go:embed isrg-root-x1.der
	isrgRootX1 string
	// SHA-256 e57b7e6f150c419102e8d5c055729ff967b9d1a829bf00cec89ca604ebf4a86f.
	//go:embed isrg-root-yr.der
	isrgRootYR string
)

// ISRG returns Let's Encrypt's RSA roots: ISRG Root X1 (expires 2035) and the
// self-signed Root YR (expires 2045), which LE also serves cross-signed by X1.
// The returned DER aliases read-only memory and must not be modified.
func ISRG() x509.Chain {
	return x509.Chain{view(isrgRootX1), view(isrgRootYR)}
}

func view(s string) []byte { return unsafe.Slice(unsafe.StringData(s), len(s)) }
