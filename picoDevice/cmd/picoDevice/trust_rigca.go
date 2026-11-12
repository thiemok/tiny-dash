//go:build rigca

package main

import (
	_ "embed"
	"unsafe"

	"github.com/soypat/lcrypto/x509"
)

// The rigca build trusts a local test CA instead of Let's Encrypt, so the
// power rig can measure HTTPS against a dev server. Its chain mirrors the
// production one (RSA-4096 roots, one cross-signed, RSA-4096 intermediate,
// RSA-2048 leaf) so the handshake costs the same. The DER files are made by
// the rig's CA script and are not committed.
var (
	//go:embed rigca/root-x1.der
	rigRootX1 string
	//go:embed rigca/root-yr.der
	rigRootYR string
)

func trustRoots() x509.Chain {
	return x509.Chain{
		unsafe.Slice(unsafe.StringData(rigRootX1), len(rigRootX1)),
		unsafe.Slice(unsafe.StringData(rigRootYR), len(rigRootYR)),
	}
}
