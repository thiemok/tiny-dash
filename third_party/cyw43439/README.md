# cyw43439 (vendored, patched)

[soypat/cyw43439](https://github.com/soypat/cyw43439), the CYW43439 WiFi
driver for the Pico W and Pico 2 W, with one patch the device needs to join
faster after its reboots between fetches.

Modules use it through a `replace` directive:

```
replace github.com/soypat/cyw43439 => ../third_party/cyw43439
```

| | |
|---|---|
| Upstream | `github.com/soypat/cyw43439` |
| Commit | `318d5aff4d24` (`v0.1.2-0.20261015151841-318d5aff4d24`) |
| License | BSD-3-Clause, see [LICENSE](LICENSE) (unchanged from upstream) |
| Upstream README | [README.upstream.md](README.upstream.md) |
| Left out | `examples/`, `cmd/`, `_legacy_cyrw/` and `stack_comparison.png` |

## Patch

`JoinOptions.Channel` limits a join to one 2.4 GHz channel through the
`join` iovar, as the pico-sdk's `cyw43_ll_wifi_join` does, instead of
`WLC_SET_SSID`, which makes the firmware scan every channel first (~0.8 s more per join).
`Device.Channel` reports the channel once joined, so a caller can remember
it. Both are in `wifi.go`; they are meant to go upstream.
