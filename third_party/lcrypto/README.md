# lcrypto (vendored, patched)

A patched subset of [soypat/lcrypto](https://github.com/soypat/lcrypto): heapless
ports of Go's crypto primitives that `tinytls` uses for TLS 1.3 on the Pico 2 W
(AES-GCM, SHA-256, X25519, and `x509.Verifier` with RSA, ECDSA and Ed25519).

Modules use it through a `replace` directive:

```
replace github.com/soypat/lcrypto => ../third_party/lcrypto
```

| | |
|---|---|
| Upstream | `github.com/soypat/lcrypto` |
| Commit | `a1194659b688` (`v0.0.0-20260926162243-a1194659b688`) |
| License | BSD-3-Clause, see [LICENSE](LICENSE) (unchanged from upstream) |
| Upstream README | [README.upstream.md](README.upstream.md) |

Upstream warns that its `x509` package was LLM-written, not machine-translated from
the standard library, and it has not been audited.

## What is included

Only the packages that `aesgcm`, `sha256`, `x25519` and `x509` need (`go list -deps`),
with their tests, plus the test helpers in `internal/lcryptotest`. Left out:
`chacha20poly1305`, `mlkem`, `p256`, `x25519mlkem768`, the generator
(`gen.go`, `internal/cmd`), and the Wycheproof vectors used only by the packages
left out (`chacha20_poly1305_*`, `mlkem_768_*`).

## Patches

1. **`go.mod`: `go 1.26.3` → `go 1.26.0`**, the workspace's version. lcrypto uses
   nothing newer.
2. **P-256 base-point table in flash** (`internal/std/nistec/p256_table.go`,
   `internal/std/nistec/p256.go`). Upstream declares the 88,064 B precomputed table
   as `var p256PrecomputedEmbed = [...]byte{...}`. TinyGo places a mutable array in
   `.data`, so it is copied into RAM at boot and takes 17% of the RP2350's SRAM.
   The patch makes it a `const` string, which stays in flash, and points
   `p256GeneratorTables` at it with `unsafe.StringData`. The table is only ever
   read. Strings are only byte aligned, and Cortex-M faults on an unaligned 64-bit
   load, so `init` copies a misaligned table the same way it already did on
   big-endian hosts. In the picoDevice build the string is 8-byte aligned, so it
   is not copied. The table is linked because `x509.Verifier` supports ECDSA P-256
   certificates.

## Re-syncing with upstream

1. `go mod download -json github.com/soypat/lcrypto@<commit>` and note its `Dir`.
2. Replace every package directory here with upstream's. Keep this README and
   `README.upstream.md`, refreshed from upstream. To check the package list, run
   `go list -deps ./aesgcm ./sha256 ./x25519 ./x509 .` in upstream's `Dir`.
3. Copy `internal/lcryptotest` and delete the vectors no included test uses.
4. Re-apply the patches. For patch 2, regenerate the string const from
   upstream's table, e.g. with a short Go program that parses the `[...]byte{}`
   literal and writes `\xNN` escapes, then add the `unsafe.StringData` and
   alignment check back into `p256.go`'s `init`.
5. Run `go test ./...` in this directory, and `pnpm nx run tinytls:build:tinygo`,
   `tinytls:test` and `tinytls:test:tinygo`.
6. Update the commit above.
