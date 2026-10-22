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
   API_HOST=192.168.1.100  # IP or hostname of the API server
   API_PORT=8080
   
   # Refresh Configuration (seconds)
   REFRESH_INTERVAL=60
   ```

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
2. **WiFi Connection**: Connects to the configured WiFi network with retry logic
3. **Main Loop**:
   - Fetches image data from the API server
   - Sends display parameters (resolution, color depth, supported colors)
   - Receives packed pixel data matching the display's framebuffer format
   - Updates the e-ink display
   - Waits for the configured refresh interval
   - Repeats

## Error Handling

- WiFi connection failures: Retries up to 5 times with 2-second delays
- API fetch failures: Logs error and keeps the previous image
- Display update failures: Logs error but continues the refresh loop
- Invalid response sizes: Validates before updating display

## Development

### Dependencies

- TinyGo (for building Pico 2 W firmware)
- github.com/soypat/cyw43439 (WiFi driver)
- github.com/thiemok/tiny-dash/inky (e-ink display library)

### Project Structure

```
picoDevice/
├── cmd/picoDevice/     # Main application
├── internal/
│   ├── config/         # Build-time configuration (auto-generated)
│   ├── wifi/           # WiFi management (future)
│   └── client/         # HTTP client (future)
├── tools/
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
