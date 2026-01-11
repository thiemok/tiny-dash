# Inky Library

TinyGo library for the Inky Impression 7.3" Spectra 6 (2025 Edition) e-ink display.

## Overview

This is a standalone Go/TinyGo library that provides a simple, auto-configuring interface for the Inky Impression 7.3" Spectra 6 display with the e640 controller. The library handles all SPI and GPIO configuration automatically, providing a zero-configuration API for easy use.

## Features

- ✅ **Zero Configuration** - Just call `NewSpectra6()` and you're ready to go
- ✅ **Auto-configured Pins** - Fixed pin assignments for the Inky PCB
- ✅ **6-Color Support** - Black, White, Red, Yellow, Blue, Green
- ✅ **Simple API** - Minimal, clean interface
- ✅ **TinyGo Native** - Built specifically for embedded hardware
- ✅ **Standalone Library** - No external dependencies

## Hardware Specifications

**Display:** Inky Impression 7.3" Spectra 6 (2025 Edition)  
**Controller:** e640  
**Resolution:** 800×480 pixels  
**Colors:** 6 (Black, White, Red, Yellow, Blue, Green)  
**Refresh Time:** ~30-40 seconds (hardware limited)  
**Interface:** SPI + GPIO control pins

## Pin Assignments

### Hard Stuff Pico to Pi Adapter Configuration

When using the Pico 2 W with the Hard Stuff Pico to Pi adapter, the pins are mapped from Raspberry Pi HAT pins to Pico GPIO:

| Function | RPi GPIO | Pico GPIO | Status |
|----------|----------|-----------|--------|
| CS | 8 (CE0) | **GP8** | ✅ Confirmed |
| DC | 22 | **GP6** | ✅ Confirmed |
| RST | 27 | **GP27** | ⚠️ Verify with hardware |
| BUSY | 17 | **GP17** | ✅ Confirmed |
| CLK | 11 | SPI0 CLK | Auto-configured |
| MOSI | 10 | SPI0 MOSI | Auto-configured |
| GND | - | GND | Ground |
| 3V3 | - | 3V3 | Power (3.3V) |

**Important Notes:**
- CS, DC, and BUSY pins have been confirmed with hardware
- RST pin (GP27) needs verification - if display doesn't initialize, try other pins
- The adapter uses Arduino pin numbering in its header, but these are the actual Pico GPIO mappings

### Direct Connection (Without Adapter)

If connecting the Inky display directly to a Pico (not using the adapter):
- CS: GP8, DC: GP22, RST: GP27, BUSY: GP17

## Installation

This library is part of the tiny-dash monorepo. To use it in your own project:

```bash
# Add as a dependency in your go.mod
go get github.com/thiemok/tiny-dash/inky
```

## Usage

### Basic Example

```go
package main

import (
    "github.com/thiemok/tiny-dash/inky/pkg/inky"
)

func main() {
    // Create display - automatically configures everything!
    display, err := inky.NewSpectra6()
    if err != nil {
        println("Error:", err.Error())
        return
    }

    // Create an image (800x480 bytes, values 0-6 for colors)
    image := make([]byte, inky.Width*inky.Height)
    
    // Fill with white
    for i := range image {
        image[i] = byte(inky.White)
    }
    
    // Set the image
    display.SetImage(image)
    
    // Refresh the display (takes ~30-40 seconds)
    display.Refresh()
}
```

### Color Constants

```go
inky.Black   // 0
inky.White   // 1
inky.Yellow  // 2
inky.Red     // 3
inky.Blue    // 5
inky.Green   // 6
```

### API Reference

#### `NewSpectra6() (*Display, error)`
Creates and initializes a new Inky Impression 7.3" Spectra 6 display. Automatically configures SPI and GPIO pins.

#### `SetImage(data []byte) error`
Sets the image buffer. Data must be `Width * Height` bytes (800×480 = 384,000 bytes), with each byte representing a color value (0-6).

#### `Refresh() error`
Updates the display with the current buffer. This operation takes approximately 30-40 seconds due to the e-ink refresh process.

#### `Clear(color Color) error`
Clears the entire display to a single color and transfers the image. You still need to call `Refresh()` to update the display.

## Building

### Build Library Only
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

### Upload Example to Pico
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

## Example Program

The included example program (`cmd/example/main.go`) displays a test pattern with 6 vertical color bars, one for each supported color. This is useful for verifying the display works correctly and demonstrating all 6 colors.

## Image Format

Images must be:
- **Size:** 800×480 pixels = 384,000 bytes
- **Format:** 1 byte per pixel
- **Values:** 0-6 representing colors (Black, White, Yellow, Red, Blue, Green)
- **Byte Order:** Row-major (left-to-right, top-to-bottom)

Example of setting a single pixel:
```go
x, y := 100, 50  // Position
color := inky.Red
image[y*inky.Width + x] = byte(color)
```

## Performance

- **Initialization:** ~1 second
- **Image Transfer:** <1 second (384 KB over SPI)
- **Display Refresh:** 30-40 seconds (hardware limitation)
- **SPI Speed:** 1 MHz
- **Memory Usage:** ~384 KB for image buffer

## Memory Considerations

The Raspberry Pi Pico 2 W has 520 KB of RAM. The image buffer uses 384 KB (~74% of RAM), leaving ~136 KB for program code and stack. This is sufficient for the display library and moderate application code.

## Troubleshooting

### Display not responding
- Check all pin connections
- Verify power supply (3.3V, sufficient current)
- Try increasing timeout values if needed

### Display shows incorrect colors
- Verify image data uses correct color values (0-6)
- Check that image size is exactly 384,000 bytes

### Build errors
- Ensure TinyGo is installed: `tinygo version`
- Check Go version: `go version` (requires 1.25+)
- Verify you're building with the correct target: `pico-w`

## Reference

This library is a Go/TinyGo port of [Pimoroni's Python inky library](https://github.com/pimoroni/inky), specifically the e640 controller implementation.

## License

See the main tiny-dash repository for license information.

## Future Work

Support for additional display controllers:
- e673
- uc8159
- ac073tc1a
- el133uf1
- jd79661, jd79668
- ssd1608, ssd1683

See `vendor/pimoroni-inky/` for reference implementations.
