package inky

import (
	"fmt"
)

// Display constants for Inky Impression 4.0" Spectra 6 (E640)
const (
	E640Width  = 400
	E640Height = 600
)

// E640 supported colors (6-color Spectra 6 display)
var e640SupportedColors = []Color{Black, White, Yellow, Red, Blue, Green}

// E640Display represents an Inky E640 4.0" Spectra 6 e-ink display
type E640Display struct {
	config InkyConfig
	buffer [E640Width * E640Height / 2]byte // Packed format: 2 pixels per byte (4 bits each)
}

// NewE640 creates and initializes an Inky Impression 4.0" Spectra 6 display (E640)
// Accepts a configured InkyConfig with all required interfaces
func NewE640(config InkyConfig) (*E640Display, error) {
	display := &E640Display{
		config: config,
		// buffer is a fixed-size array, allocated as part of the E640Display struct
	}

	// Configure pins for this display
	if err := config.CS.Configure(PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure CS pin: %w", err)
	}
	if err := config.DC.Configure(PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure DC pin: %w", err)
	}
	if err := config.RST.Configure(PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure RST pin: %w", err)
	}
	if err := config.BUSY.Configure(PinInput); err != nil {
		return nil, fmt.Errorf("failed to configure BUSY pin: %w", err)
	}

	// Set initial pin states
	config.CS.Set(true)  // CS idle high
	config.DC.Set(false) // DC default low
	config.RST.Set(true) // RST idle high

	// Perform hardware initialization
	if err := display.init(); err != nil {
		return nil, fmt.Errorf("display initialization failed: %w", err)
	}

	return display, nil
}

// init initializes the e640 controller
// Based on Pimoroni's Python implementation
func (d *E640Display) init() error {
	// Hardware reset
	reset(d.config.RST)

	// Wait for display to be ready
	if !busyWait(d.config.BUSY, 5) {
		return fmt.Errorf("timeout waiting for display ready after reset")
	}

	// Send initialization sequence from e640.py
	// Magic initialization command
	d.sendCommand(0xAA, []byte{0x49, 0x55, 0x20, 0x08, 0x09, 0x18})

	// Power setting
	d.sendCommand(cmdPWR, []byte{0x3F})

	// Panel setting register
	d.sendCommand(cmdPSR, []byte{0x5F, 0x69})

	// Booster soft start settings
	d.sendCommand(cmdBTST1, []byte{0x40, 0x1F, 0x1F, 0x2C})
	d.sendCommand(cmdBTST3, []byte{0x6F, 0x1F, 0x1F, 0x22})
	d.sendCommand(cmdBTST2, []byte{0x6F, 0x1F, 0x17, 0x17})

	// Power off sequence setting
	d.sendCommand(cmdPOFS, []byte{0x00, 0x54, 0x00, 0x44})

	// TCON setting
	d.sendCommand(cmdTCON, []byte{0x02, 0x00})

	// PLL control
	d.sendCommand(cmdPLL, []byte{0x08})

	// VCOM and data interval setting
	d.sendCommand(cmdCDI, []byte{0x3F})

	// Resolution setting (400x600)
	// 0x0190 = 400, 0x0258 = 600
	d.sendCommand(cmdTRES, []byte{0x01, 0x90, 0x02, 0x58})

	// Power saving
	d.sendCommand(cmdPWS, []byte{0x2F})

	// VCOM DC setting
	d.sendCommand(cmdVDCS, []byte{0x01})

	return nil
}

// sendCommand sends a command with optional data to the display
func (d *E640Display) sendCommand(command byte, data []byte) {
	sendCommand(d.config.SPI, d.config.CS, d.config.DC, command, data)
}

// GetFramebuffer returns a Framebuffer for pixel-level access to the display buffer
// The Framebuffer provides SetPixel/GetPixel methods that handle the packed format internally
// No allocation - returns a lightweight wrapper around the existing buffer
func (d *E640Display) GetFramebuffer() Framebuffer {
	return Framebuffer{
		data:   d.buffer[:],
		width:  E640Width,
		height: E640Height,
	}
}

// Update transfers the framebuffer to the display and triggers a refresh
// This takes approximately 40 seconds to complete for the E640
func (d *E640Display) Update() error {
	// Power on
	d.sendCommand(cmdPON, nil)
	if !busyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for power on")
	}

	// Send packed image data to display
	d.sendCommand(cmdDTM1, d.buffer[:])

	// Second setting of BTST2 register (from Python implementation)
	d.sendCommand(cmdBTST2, []byte{0x6F, 0x1F, 0x17, 0x47})

	// Display refresh
	d.sendCommand(cmdDRF, []byte{0x00})

	// Wait for refresh to complete (E640 takes ~40 seconds)
	println("Refreshing display (this takes ~40 seconds)...")
	if !busyWait(d.config.BUSY, 45) {
		return fmt.Errorf("timeout waiting for display refresh")
	}

	// Power off
	d.sendCommand(cmdPOF, []byte{0x00})
	if !busyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for power off")
	}

	return nil
}

// Clear fills the framebuffer with a single color
func (d *E640Display) Clear(color Color) {
	// Validate color is supported
	if !d.SupportsColor(color) {
		panic(fmt.Sprintf("E640 does not support color %s (value %d)", color.String(), color))
	}

	// Pack the color: 2 pixels per byte
	packed := byte(color<<4) | byte(color)
	for i := range d.buffer {
		d.buffer[i] = packed
	}
}

// Width returns the display width in pixels
func (d *E640Display) Width() int {
	return E640Width
}

// Height returns the display height in pixels
func (d *E640Display) Height() int {
	return E640Height
}

// SupportedColors returns the list of colors supported by this display
func (d *E640Display) SupportedColors() []Color {
	return e640SupportedColors
}

// SupportsColor checks if the display supports a specific color
func (d *E640Display) SupportsColor(color Color) bool {
	for _, c := range e640SupportedColors {
		if c == color {
			return true
		}
	}
	return false
}
