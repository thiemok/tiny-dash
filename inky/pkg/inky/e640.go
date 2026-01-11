package inky

import (
	"fmt"
	"time"
)

// Display constants for Inky Impression 4.0" Spectra 6 (E640)
const (
	E640Width  = 400
	E640Height = 600
)

// E640Display represents an Inky E640 4.0" Spectra 6 e-ink display
type E640Display struct {
	spi    *hardwareSPI
	gpio   *hardwareGPIO
	buffer [E640Width * E640Height / 2]byte // Packed format: 2 pixels per byte (4 bits each)
}

// NewE640 creates and initializes an Inky Impression 4.0" Spectra 6 display (E640)
// Automatically configures SPI and GPIO with correct pins
func NewE640() (*E640Display, error) {
	display := &E640Display{
		spi:  initSPI(),
		gpio: initGPIO(),
		// buffer is a fixed-size array, allocated as part of the E640Display struct
	}

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
	d.gpio.reset()

	// Wait for display to be ready
	if !d.gpio.busyWait(5) {
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
	d.gpio.setCS(false)
	d.gpio.setDC(false)
	time.Sleep(300 * time.Microsecond)
	d.spi.transfer([]byte{command})

	if data != nil && len(data) > 0 {
		d.gpio.setDC(true)
		d.spi.transfer(data)
	}

	d.gpio.setCS(true)
	d.gpio.setDC(false)
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
	if !d.gpio.busyWait(1) {
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
	if !d.gpio.busyWait(45) {
		return fmt.Errorf("timeout waiting for display refresh")
	}

	// Power off
	d.sendCommand(cmdPOF, []byte{0x00})
	if !d.gpio.busyWait(1) {
		return fmt.Errorf("timeout waiting for power off")
	}

	return nil
}

// Clear fills the framebuffer with a single color
func (d *E640Display) Clear(color Color) {
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
