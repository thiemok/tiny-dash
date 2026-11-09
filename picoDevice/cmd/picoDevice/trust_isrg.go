//go:build !rigca

package main

import (
	"github.com/soypat/lcrypto/x509"
	"github.com/thiemok/tiny-dash/tinytls/roots"
)

// trustRoots returns the roots the API's certificate must chain to.
func trustRoots() x509.Chain { return roots.ISRG() }
