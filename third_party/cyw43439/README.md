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

Init and Join wait as pico-sdk's `cyw43_ll_wifi_on` does (50 ms after
`country`, `event_msgs` and `WLC_UP`; 3 ms before the PMK) instead of
100 ms after every step, and `wait_for_join` polls every 20 ms instead of
270 ms. Two waits stay at 100 ms because measurements needed them: after the
A-MPDU settings (without it the download ran ~0.45 s slower) and after
`WLC_SET_BAND` (without it ~1 in 10 joins failed with "no network").

`pollForIoctl` polls for the ioctl reply every 250 µs instead of 10 ms (same
~100 ms timeout); `waitForCredit` keeps 10 ms, because polling it faster made
downloads stall on the data path. With the faster replies the wait after
`WLC_SET_BAND` had to grow to 200 ms to keep joins from failing.

`whd/whd_test.go`: `TestParseAsyncEvent` parses with `binary.BigEndian`, the
order `ParseAsyncEvent` documents and its expected values assume; upstream
passes `binary.LittleEndian` and fails.

## In this repo's Nx workspace

The module is an Nx project like the others, so CI runs its lint and test
targets. Its `generate` target is a no-op (`project.json`): the generated
`*_string.go` files are committed, and regenerating them needs `stringer`.
