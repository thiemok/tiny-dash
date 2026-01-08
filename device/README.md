# Device Module

TinyGo application for the embedded device controller.

## Overview

This module contains the firmware for the embedded device that manages the e-ink display, button input, and communication with the renderer service.

## Module Type

- **Go Module**: TinyGo (compatible with embedded hardware)
- **Build System**: Nx with shell scripts
- **Dependencies**: Will depend on `eink` module (not yet created)

## Current Status

**Hello-World Implementation**: Basic TinyGo program with serial logging.

## Target Hardware

- **Raspberry Pi Pico 2 W** - Exclusively supported

## Building

```bash
# Build for Pico 2 W
nx build device

# Or using the direct path to nx
./node_modules/.bin/nx build device

# Build output location
# device/dist/device.uf2
```

**Note**: Make sure to run `nvm use 22` first to activate the correct Node.js version.

## Flashing

### Raspberry Pi Pico 2 W

1. Hold BOOTSEL button while connecting USB
2. Copy `device/dist/device.uf2` to the RPI-RP2 drive
3. Device will reboot automatically

## Viewing Serial Output

```bash
# Using minicom
minicom -D /dev/ttyACM0 -b 115200

# Using screen
screen /dev/ttyACM0 115200

# Or on macOS
screen /dev/cu.usbmodem* 115200
```

## Expected Output

```
Hello from tiny-dash device!
Target hardware initialized
Loop iteration: 0
Loop iteration: 1
Loop iteration: 2
...
```

## Hardware Specifications

### Raspberry Pi Pico 2 W

- **Microcontroller**: RP2350 dual-core ARM Cortex-M33 @ 150MHz
- **RAM**: 520KB SRAM
- **Flash**: 4MB
- **Connectivity**: WiFi 802.11n (2.4GHz)
- **GPIO**: 26 multifunction GPIO pins
- **Interfaces**: SPI, I2C, UART, USB 1.1
- **Power**: 1.8-5.5V (USB or external)

## TinyGo Target Note

**Important**: This project currently uses the `pico-w` target for TinyGo, NOT `pico-2-w`.

The Raspberry Pi Pico 2 W is a newer board and TinyGo doesn't have a specific `pico-2-w` target yet. However, the Pico 2 W is backward compatible with the original Pico W for basic functionality.

**Current Setup**:
- Target: `pico-w`
- Works with Pico 2 W hardware
- Basic functionality is fully supported

**Future**: Once TinyGo adds official Pico 2 W support with a `pico-2-w` target, we will update the build script to use it.

## Next Steps

- Integrate eink display driver
- Add button input handling
- Implement HTTP client for renderer communication
- Add image fetching and display logic
- Implement caching and error handling
- Configure WiFi connectivity

## Documentation

See [tasks/device.md](../tasks/device.md) for detailed implementation planning.
