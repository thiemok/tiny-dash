# picoDevice

TinyGo application for Raspberry Pi Pico 2 W that displays dashboard content on an e-ink display using the Inky library.

## Overview

The picoDevice application:
- Connects to WiFi using the CYW43439 wireless chip
- Fetches dashboard images from the API server
- Displays images on an e-ink display (auto-detected via EEPROM)
- Refreshes the display at configurable intervals

## Hardware Requirements

- Raspberry Pi Pico 2 W
- Pimoroni Inky Impression e-ink display (with EEPROM)
- Pico-to-Pi adapter board

## Configuration

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` with your settings:
   ```bash
   # WiFi Configuration
   WIFI_SSID=your_wifi_network_name
   WIFI_PASSWORD=your_wifi_password
   
   # API Configuration
   API_URL=http://192.168.1.100:8080
   
   # Refresh Configuration (seconds)
   REFRESH_INTERVAL=60
   ```

### API_URL

`API_URL` is the base URL of the API server: `scheme://host[:port][/path-prefix]`.

- The port defaults to 80 for `http` and 443 for `https`.
- An optional path prefix is prepended to `/api/dashboard/image`
  (`http://10.0.0.5/tiny-dash` fetches `/tiny-dash/api/dashboard/image?...`).
- The `Host` header is the configured host, with `:port` only for non-default ports.
- Userinfo, query strings and fragments are rejected.

The URL is parsed by `netkit/pkg/apiurl` at startup; an invalid URL halts the device
with an `invalid API_URL` log line before WiFi is brought up.

### DNS

A host name in `API_URL` is resolved on the device before the first fetch, with a query
encoded once by `netkit/pkg/dnsquery` (lneto's `dns` wire format) and sent over a reused UDP
conn to the DNS server from the DHCP lease (logged as `dns server ... source=dhcp` after
each WiFi connect). `DNS_SERVER` (an IPv4 literal) is used only if DHCP offers none. An IP
literal (e.g. `http://192.168.1.100:8080` for a local `nx run api:serve`) skips DNS entirely.

Each lookup is logged as `✓ API host resolved` with the address and the time it took; debug
builds add the lookup's heap allocations (a re-lookup allocates only inside lneto's
`DialUDP`). The record's TTL is not
used yet, so the address is looked up again after an hour, after two consecutive failed TCP dials, or after a WiFi reconnect. If that
lookup fails, the previous address is kept (`dns lookup failed ... usingPrevious=true`).
The caching policy lives in `netkit/pkg/resolve`.

Current limitations:

- Only A records for the host name itself are used, so the lookup fails for a host name
  that the resolver answers with a CNAME chain.
- **Redirects** (3xx) are not followed. A redirect to https (e.g. a reverse proxy's
  http→https redirect) is logged as
  `server redirected to https; set API_URL scheme to https` with the `Location`.

### Time (NTP)

Certificate validation (https, THI-32) needs the wall time, and the device boots at
1970-01-01. After WiFi comes up, before the first fetch, the device syncs a wall clock
(`netkit/pkg/wallclock`: the monotonic time since boot plus an offset) with the NTP server
`NTP_SERVER` (default `pool.ntp.org`, resolved through the DNS resolver above before every
sync; an IPv4 literal skips DNS). The request and response are SNTPv4 (`netkit/pkg/ntpquery`):
the response must echo a random nonce, and kiss-of-death, unsynchronized (leap alarm,
stratum 16) and malformed answers are refused. The conn borrows the fetch conn's idle
receive buffer, as the DNS lookups do.

- Each sync logs `ntp server` (host, ip) and then `✓ clock synced` with the stratum, the
  round trip, the `offset` the clock moved by and, in debug builds, the sync's heap
  allocations. Log lines
  carry the wall time from then on; before the first sync their time is the time since boot
  as `1970-01-01T...`.
- The clock re-syncs every 12 h and after every WiFi reconnect. A re-sync that moves it by
  more than 10 s logs `large clock step on re-sync`.
- A failed sync logs `ntp sync failed` with the stage (debug builds add its heap
  allocations) and is
  retried at the start of a later refresh cycle, backing off from 30 s to 15 min.
  Plain-HTTP fetches carry on meanwhile; https fetches wait (see [HTTPS](#https)). Until the
  first success the clock reports not synced (`Clock.Synced`, and `Clock.Nanotime` returns 0,
  so certificate checks fail closed).
- The build time is the clock's floor: `tools/build.sh` and the `upload` target pass
  `-ldflags "-X main.buildUnix=$(date -u +%s)"`, and no NTP answer can set the clock earlier.
  The `wall clock floor` line at boot shows it.

DHCP option 42 (NTP servers) is not used: lneto requests and parses it but does not expose
it, and the LAN's DHCP server does not offer it.

### HTTPS

An `https://` API_URL is fetched over TLS 1.3 with the `tinytls` module
(`TLS_AES_128_GCM_SHA256`, X25519) on top of the same TCP conn. The SNI and the `Host`
header are the host name, and the port defaults to 443. Plain `http://` stays supported for
local development and does not link any TLS state into the fetch path.

- **Validation.** Every handshake validates the chain up to an embedded root (ISRG Root X1
  or Root YR, Let's Encrypt's RSA roots; `tinytls/roots`), the host name, the validity
  period at the NTP time, and the server's CertificateVerify signature. The firmware has no
  way to skip it. A server certificate from another CA needs its root added to
  `tinytls/roots`.
- **Clock.** Before the first NTP sync, an https fetch is refused instead of validating
  against a wrong time: `image fetch failed stage=clock error="wall clock not synced; ..."`.
  The display keeps the last image, and the fetch goes ahead in the first cycle after a sync.
- **Failures.** A certificate that does not validate logs `image fetch failed stage="tls
  verify"` with the reason, e.g. `x509: certificate is not valid for the expected name`
  or `x509: certificate signed by unknown authority`. It
  is not retried within the cycle and does not count towards a WiFi reconnect. A handshake
  that does not finish within 10 s is retried like a missing response header. Either way
  the display keeps the last image.
- **Entropy.** TinyGo's `crypto/rand` has no source on the RP2350, so the key shares come
  from `tinytls/drbg` (a SHA-256 DRBG), seeded at boot from the RP2350 TRNG with its health
  tests enabled (`✓ TRNG seeded the TLS DRBG`) and fed a fresh TRNG sample before every
  handshake.
- **Teardown.** After every fetch the device sends `close_notify`, wipes the session
  secrets and closes the TCP conn. The TLS state (39.5 KB: a 16.6 KB record buffer, as the
  server sends full 16 KiB records and ignores `max_fragment_length`, and the 19 KB
  certificate verifier) is allocated once at startup and reused, so a fetch does not
  allocate. The 8 KB handshake buffer is borrowed from the framebuffer, which the body
  overwrites anyway.
- **Timing.** `display updated successfully` logs the `handshake` and `body` times: about
  250 ms and 1.1 s for the 192,000 B image.
- **Stack.** The handshake runs on the main goroutine. It uses about 4.9 KB of the 8 KB
  `-stack-size` (`main stack headroom=3252` in a debug build with `MEM_TEST_GC=true`).

### Migrating from API_HOST/API_PORT

Older `.env` files used `API_HOST` and `API_PORT`. Replace them with a single line:

```bash
# before
API_HOST=192.168.1.100
API_PORT=8080
# after
API_URL=http://192.168.1.100:8080
```

Until you do, `tools/configure.sh` derives `API_URL=http://$API_HOST:$API_PORT` and
prints a deprecation warning, so existing `.env` files keep building.

The URL parser lives in the `netkit` Go module. If your `go.work` predates it, add it
with `go work use ./netkit` (Orca worktree setup does this automatically).

## Building

The build process:
1. Reads configuration from `.env`
2. Generates `internal/config/generated.go` with build-time constants
3. Compiles the firmware using TinyGo

To build the firmware:

```bash
bash tools/build.sh
```

This creates `dist/picoDevice.uf2` ready for flashing.
`bash tools/build.sh flash`, which the `upload` target runs, builds with the same
flags and flashes the result with `tinygo flash`.

### Debug builds

The default build is the release firmware. Measurement and test code (heap
statistics in the logs, test knobs) only compiles into a debug build, tagged
`picodebug`:

```bash
pnpm nx run picoDevice:build:debug   # dist/picoDevice.uf2 with -tags=picodebug
pnpm nx run picoDevice:upload:debug  # flash a debug build
```

## Flashing

1. Hold the BOOTSEL button on the Pico 2 W
2. Connect it to your computer via USB (while still holding BOOTSEL)
3. Release BOOTSEL - the Pico will appear as a USB mass storage device (RPI-RP2)
4. Copy `dist/picoDevice.uf2` to the RPI-RP2 drive
5. The device will automatically reboot and start running

## Monitoring

To view serial output from the device:

```bash
tinygo monitor
```

Or use any serial terminal at 115200 baud.

Logs go through a 1 KiB [logkit](../logkit) ring buffer, not straight to `machine.Serial`.
TinyGo's USB-CDC `Write` blocks until the host reads, so a port with DTR asserted but no reader
would otherwise stall every goroutine that logs. Output is discarded while no reader holds DTR.
If records were dropped, a `logkit: dropped=<bytes>` line with the running total is printed.
A panic in `main` (e.g. `util.RequireNoError` at startup) is first logged as `level=ERROR msg=panic`
and the ring is flushed, so the message and the records before it reach the host.

TinyGo advertises USB remote wakeup but never signals it. Linux autosuspends the device after
2 s without serial traffic, even while a reader has the port open, and output would then stop
until the port is reopened. `usbWaker` (`cmd/picoDevice/usbwake.go`), a hook of the log drainer,
holds output back and signals the wakeup whenever output is pending while the bus is suspended. Host-side alternative: disable autosuspend
for the Pico, e.g. with a udev rule
`ACTION=="add", SUBSYSTEM=="usb", ATTR{idVendor}=="2e8a", ATTR{idProduct}=="000a", TEST=="power/control", ATTR{power/control}="on"`.

For a non-interactive capture:

```bash
stty -F /dev/ttyACM0 115200 raw -echo; timeout 300 cat /dev/ttyACM0 | tee device.log
```

`tools/serialstress` is a small firmware that reproduces the stall; `tools/build.sh` builds it into
`dist/serialstress.uf2`. Each line reports how long the previous `Write` blocked. Build it with
`-ldflags="-X main.mode=ring"` to compare with logkit.

## How It Works

1. **Startup**: Initializes hardware and detects the e-ink display
2. **WiFi Connection**: Joins the configured WiFi network and runs DHCP, retrying with backoff
3. **Main Loop**:
   - Fetches image data from the API server
   - Sends display parameters (resolution, color depth, supported colors)
   - Receives packed pixel data matching the display's framebuffer format
   - Updates the e-ink display
   - Waits for the configured refresh interval
   - Repeats

## Error Handling

- WiFi connection failures: the CYW43439 is initialized once; every attempt joins and runs DHCP
  and gateway ARP. (Re-running `Device.Init` fails the SPI test on the current driver, so a
  failed attempt re-joins instead of power-cycling the chip.) A failed attempt is logged as `wifi connect failed` with the
  stage and error, then retried after a capped exponential backoff (1 s doubling to 60 s, each
  delay jittered into [d/2, d) by the RP2350 RNG, see `netkit/pkg/backoff`). This repeats until
  it succeeds; the device never gives up.
- Link loss: before each refresh the loop checks the link state the chip reports (LINK,
  DEAUTH and DISASSOC events drop it) and reconnects as above. Three consecutive network
  fetch failures also trigger a reconnect. The display keeps the last image meanwhile.
- API fetch failures: Logs error and keeps the previous image. The fetch connection uses
  lneto's RFC 6298 retransmission timer, so a lost request segment is resent (after 1 s,
  then backing off) instead of hanging the fetch. A response header that does not arrive
  within 10 s, or a body that makes no progress for 5 s or ends early, is retried up to
  twice with a 500 ms–2 s jittered backoff before the cycle counts as failed.
- Display update failures: Logs error but continues the refresh loop
- Invalid response sizes: Validates before updating display

### WiFi test knobs

These `.env` settings force join failures for hardware testing. Only debug builds read them
(`pnpm nx run picoDevice:upload:debug`, see [Debug builds](#debug-builds)); release builds
ignore them. They default to 0 (disabled).

- `WIFI_TEST_FAIL_JOINS=N`: the first N joins at boot use a missing SSID, so the chip
  reports a genuine join failure (`join:SET_SSID failed`).
- `WIFI_TEST_OUTAGE_AFTER_CYCLE=K` with `WIFI_TEST_OUTAGE_FAIL_JOINS=M`: after refresh cycle
  K the chip joins the missing SSID, which makes it leave the access point like an outage,
  and the next M joins fail the same way.

The cyw43439 driver logs at Warn, because its Info-level `rxEvent` records allocate on
every join. To see why a join failed, raise the driver logger in `main.go` to Info: each
join then records `rxEvent` lines whose `status` and `reason` say why (status 3 = no
network found, 1 = failure).

### Memory test knobs

Like the WiFi test knobs, these only take effect in debug builds
(`pnpm nx run picoDevice:upload:debug`). The `cycle` line's `mallocs`, `bytes` and `heapAlloc`
are debug-only too: a release build logs just `n` and `ok`.

- `NTP_TEST_SYNC_INTERVAL=N`: re-sync the clock every N seconds instead of every 12 hours,
  to exercise the sync path many times in one session.
- `MEM_TEST_GC=true`: run the GC before each cycle's `runtime.ReadMemStats`, so the
  `heapAlloc` of the `cycle` line is the live heap and a leak shows as steady growth. It also
  logs `main stack headroom=N`, the bytes of the main goroutine's stack (which runs the
  fetch and the TLS handshake) that were never written, to size `-stack-size`.

## Development

### Dependencies

- TinyGo (for building Pico 2 W firmware)
- github.com/soypat/cyw43439 (WiFi driver)
- github.com/thiemok/tiny-dash/inky (e-ink display library)
- github.com/thiemok/tiny-dash/netkit (API URL parsing, retry backoff, DNS query and caching policy, NTP query, wall clock and sync schedule)

### Project Structure

```
picoDevice/
├── cmd/picoDevice/     # Main application
├── internal/
│   ├── config/         # Build-time configuration (auto-generated)
│   ├── wifi/           # WiFi management (future)
│   └── client/         # HTTP client (future)
├── tools/
│   ├── configure.sh    # Renders .env into internal/config/generated.go
│   └── build.sh        # Build script
├── .env.example        # Configuration template
└── README.md           # This file
```

## Nx Commands

Using the Nx workspace:

```bash
# Build firmware
nx build picoDevice

# Build debug firmware (-tags=picodebug)
nx run picoDevice:build:debug

# Flash to device (requires device in BOOTSEL mode)
nx upload picoDevice

# Monitor serial output
nx monitor picoDevice
