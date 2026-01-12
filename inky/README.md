# Inky Library

TinyGo library for Pimoroni Inky e-ink displays with hardware abstraction and auto-detection support.

## Overview

This is a standalone Go/TinyGo library that provides a clean, hardware-abstracted interface for Pimoroni Inky e-ink displays. The library supports multiple display types and can automatically detect the connected display via EEPROM.

## Features

- ✅ **Hardware Abstraction** - Works with any hardware through interface-based design
- ✅ **Auto-Detection** - Automatically identifies display type via EEPROM
- ✅ **Multiple Displays** - Support for various Inky display models (E640, E673, and more coming)
- ✅ **Color Validation** - Displays report supported colors, with runtime validation
- ✅ **6-Color Support** - Black, White, Red, Yellow, Blue, Green (Spectra 6 displays)
- ✅ **Simple API** - Minimal, clean interface with framebuffer access
- ✅ **TinyGo Native** - Built specifically for embedded hardware
- ✅ **Zero Dependencies** - No external dependencies beyond TinyGo

## Supported Displays

### Currently Implemented
- **E673** - Spectra 6 7.3" 800×480 (variant 22)
- **E640** - Spectra 6 4.0" 400×600 (variant 25)

### Coming Soon
- InkyPHAT - 212×104 (variants 1, 4, 5)
- InkyWHAT - 400×300 (variants 2, 3, 6, 7, 8)
- InkyPHAT_SSD1608 - 212×104 (variants 10, 11, 12)
- InkyWHAT_SSD1683 - 400×300 (variants 17, 18, 19)
- InkyUC8159 - 7-color displays (variants 14, 15, 16)
- InkyAC073TC1A - 7-color 800×480 (variant 20)
- InkyEL133UF1 - Spectra 6 13.3" 1600×1200 (variant 21)
- InkyJD79661 - pHAT 250×122 (variant 23)
- InkyJD79668 - wHAT 400×300 (variant 24)

## Hardware Abstraction

The library uses interfaces for hardware access, allowing it to work with different hardware backends:

- **Pin Interface** - GPIO operations (configure, set, get)
- **I2C Interface** - For EEPROM reading
- **SPI Interface** - For display communication

### Included Adapters

**Pico2 + PicoToPi HAT Adapter**
- For Raspberry Pi Pico 2 W with Hard Stuff Pico-to-Pi adapter
- Pre-configured pin mappings matching Pimoroni's specifications
- Located in `adapters/pico2_picotopi.go`

## Installation

```bash
go get github.com/thiemok/tiny-dash/inky
```

## Usage

### Auto-Detection (Recommended)

The easiest way to use the library is with auto-detection:

```go
package main

import (
    "github.com/thiemok/tiny-dash/inky/pkg/inky"
    "github.com/thiemok/tiny-dash/inky/pkg/inky/adapters"
)

func main() {
    // Configure hardware adapter
    hardware, err := adapters.NewPico2PicoToPiHardware()
    if err != nil {
        panic(err)
    }

    // Auto-detect and initialize display
    display, err := inky.Auto(*hardware)
    if err != nil {
        panic(err)
    }

    // Use the display
    display.Clear(inky.White)
    display.Update()
}
```

### Manual Display Selection

If you know your display type or EEPROM reading fails:

```go
// For 7.3" Spectra 6 (E673)
display, err := inky.NewE673(*hardware)

// For 4.0" Spectra 6 (E640)
display, err := inky.NewE640(*hardware)
```

### Working with the Framebuffer

```go
// Get framebuffer
fb := display.GetFramebuffer()

// Draw pixels
for y := 0; y < display.Height(); y++ {
    for x := 0; x < display.Width(); x++ {
        fb.SetPixel(x, y, inky.Black)
    }
}

// Update display (transfer + refresh)
display.Update()
```

### Color Support

Check which colors a display supports:

```go
// Get list of supported colors
colors := display.SupportedColors()

// Check if a specific color is supported
if display.SupportsColor(inky.Blue) {
    fb.SetPixel(x, y, inky.Blue)
}
```

### Available Colors

```go
inky.Black   // 0
inky.White   // 1
inky.Yellow  // 2
inky.Red     // 3
inky.Blue    // 5
inky.Green   // 6
```

Note: Not all displays support all colors. E640 and E673 support all 6 colors (Spectra 6), while older displays may only support 3 colors (black, white, red/yellow).

## API Reference

### Display Interface

All display types implement this interface:

```go
type Display interface {
    GetFramebuffer() Framebuffer
    Update() error
    Clear(color Color)
    Width() int
    Height() int
    SupportedColors() []Color
    SupportsColor(color Color) bool
}
```

### Key Functions

- `Auto(config HardwareConfig) (Display, error)` - Auto-detect and initialize display
- `NewE673(config HardwareConfig) (*E673Display, error)` - Create E673 display
- `NewE640(config HardwareConfig) (*E640Display, error)` - Create E640 display
- `ReadEEPROM(i2c I2C) (*EEPROMData, error)` - Read display EEPROM

### Hardware Configuration

```go
type HardwareConfig struct {
    SPI  SPI  // SPI bus for display
    I2C  I2C  // I2C bus for EEPROM
    CS   Pin  // Chip Select
    DC   Pin  // Data/Command
    RST  Pin  // Reset
    BUSY Pin  // Busy signal
}
```

## Building

### Build Library

```bash
nx build inky
# or
cd inky && bash tools/build.sh
```

### Build Example Program

```bash
nx build-example inky
# or
cd inky && bash tools/build.sh example
```

Output: `inky/dist/example.uf2`

### Upload to Pico

```bash
nx upload inky
```

Or manually:
1. Hold BOOTSEL button while connecting Pico 2 W via USB
2. Copy `inky/dist/example.uf2` to the RPI-RP2 drive
3. Device will reboot and run the example

### Monitor Serial Output

```bash
nx monitor inky
# or
tinygo monitor
```

## Example Programs

### Display Example (`cmd/example`)

Displays a test pattern with 6 vertical color bars demonstrating all supported colors.

### EEPROM Reader (`cmd/eeprom-reader`)

Reads and displays EEPROM information from the connected display:
- Display model and variant
- Resolution
- Color type
- PCB version
- Raw EEPROM hex dump

## Image Format

Images use a packed format for memory efficiency:
- **Format:** 4 bits per pixel (2 pixels per byte)
- **Values:** 0-6 representing colors
- **Access:** Use `Framebuffer.SetPixel()` and `GetPixel()` methods
- **Layout:** Row-major (left-to-right, top-to-bottom)

## Performance

- **Initialization:** ~1 second
- **Image Transfer:** <1 second
- **Display Refresh:** 30-45 seconds (hardware limitation, varies by model)
- **SPI Speed:** 1 MHz
- **Memory Usage:** ~50% of Pico RAM for large displays (800×480)

## Creating Custom Adapters

To use with different hardware, implement the interfaces:

```go
type Pin interface {
    Configure(mode PinMode) error
    Set(high bool)
    Get() bool
}

type I2C interface {
    Tx(addr uint16, w, r []byte) error
}

type SPI interface {
    Tx(w, r []byte) error
}
```

See `adapters/pico2_picotopi.go` for a reference implementation.

## Troubleshooting

### Display Not Responding
- Check all pin connections
- Verify power supply (3.3V, sufficient current)
- Ensure correct adapter for your hardware

### EEPROM Read Fails
- Confirm I2C is properly configured
- Check I2C pins (SDA, SCL)
- Verify display is powered

### Incorrect Colors
- Check if color is supported: `display.SupportsColor(color)`
- Verify you're using the correct display type

### Build Errors
- Ensure TinyGo is installed: `tinygo version`
- Check Go version: `go version` (requires 1.21+)
- Verify target: `pico` or `pico-w`

## Reference

This library is a Go/TinyGo port of [Pimoroni's Python inky library](https://github.com/pimoroni/inky).

## License

See the main tiny-dash repository for license information.
