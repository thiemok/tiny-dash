package ssd1608

import (
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
)

// Display constants for Inky pHAT SSD1608 (250x122)
const (
	Width       = 250
	Height      = 122
	Overscan    = 6 // Y offset in physical display memory
	ColorDepth  = 2
	SSD1608Cols = 136 // Physical display columns (rotated, with offset)
	SSD1608Rows = 250 // Physical display rows (rotated)
)

// SSD1608Display represents an Inky pHAT 2.13" e-ink display with SSD1608 controller
// This is a 3-color display supporting common.Black, common.White, and common.Red or common.Yellow
type SSD1608Display struct {
	config             common.InkyConfig
	colorType          string // "red", "yellow", or "black"
	borderColor        common.Color
	bufferBW           common.Framebuffer // 1-bit packed: common.Black/common.White buffer for hardware
	bufferColor        common.Framebuffer // 1-bit packed: common.Red/common.Yellow buffer for hardware
	common.Framebuffer                    // combined 4 color buffer exposed to consumers
}

// New creates and initializes an Inky pHAT 2.13" display with SSD1608 controller (250x122)
// colorType should be "red", "yellow", or "black" to match the physical display
// UNTESTED: This implementation has not been tested on physical hardware
func New(config common.InkyConfig, colorType string) (*SSD1608Display, error) {
	// Validate color type
	if colorType != "red" && colorType != "yellow" && colorType != "black" {
		return nil, fmt.Errorf("invalid color type %q, must be 'red', 'yellow', or 'black'", colorType)
	}

	display := &SSD1608Display{
		config:      config,
		colorType:   colorType,
		borderColor: common.Black,
		Framebuffer: common.NewFramebuffer(Width, Height, ColorDepth),
		// Physical display is rotated 90deg and slightly higher resolution
		bufferBW:    common.NewFramebuffer(SSD1608Rows, SSD1608Cols, ColorDepth/2),
		bufferColor: common.NewFramebuffer(SSD1608Rows, SSD1608Cols, ColorDepth/2),
	}

	// Configure pins
	if err := config.CS.Configure(common.PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure CS pin: %w", err)
	}
	if err := config.DC.Configure(common.PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure DC pin: %w", err)
	}
	if err := config.RST.Configure(common.PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure RST pin: %w", err)
	}
	if err := config.BUSY.Configure(common.PinInput); err != nil {
		return nil, fmt.Errorf("failed to configure BUSY pin: %w", err)
	}

	// Set initial pin states
	config.CS.Set(true)
	config.DC.Set(false)
	config.RST.Set(true)

	// Perform hardware initialization
	if err := display.init(); err != nil {
		return nil, fmt.Errorf("display initialization failed: %w", err)
	}

	return display, nil
}

// init initializes the SSD1608 controller
func (d *SSD1608Display) init() error {
	// Hardware reset
	common.Reset(d.config.RST)

	// Send soft reset command
	d.sendCommand(cmdSwReset, nil)

	// Wait for display to be ready (5 second timeout)
	if !common.BusyWait(d.config.BUSY, 5) {
		return fmt.Errorf("timeout waiting for display ready after reset")
	}

	return nil
}

// sendCommand sends a command with optional data to the display
func (d *SSD1608Display) sendCommand(command byte, data []byte) {
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, command, data)
}

// Update transfers the framebuffer to the display and triggers a refresh
func (d *SSD1608Display) Update() error {
	// Convert 4-bit framebuffer to dual 1-bit buffers
	d.convertFramebufferToDualBuffers()

	// Setup display for update
	// Based on Python _update() method

	// Driver Control
	rowsLow := byte((SSD1608Rows - 1) & 0xFF)
	rowsHigh := byte(((SSD1608Rows - 1) >> 8) & 0xFF)
	d.sendCommand(cmdDriverControl, []byte{rowsLow, rowsHigh, 0x00})

	// Set dummy line period
	d.sendCommand(cmdWriteDummy, []byte{0x1B})

	// Set Gate Line Width
	d.sendCommand(cmdWriteGateLine, []byte{0x0B})

	// Data entry mode (scan direction leftward and downward)
	d.sendCommand(cmdDataMode, []byte{0x03})

	// Set RAM X start and end position
	d.sendCommand(cmdSetRAMXPos, []byte{0x00, byte(SSD1608Cols/8) - 1})

	// Set RAM Y start and end position
	d.sendCommand(cmdSetRAMYPos, []byte{0x00, 0x00, rowsLow, rowsHigh})

	// VCOM Voltage
	d.sendCommand(cmdWriteVCOM, []byte{0x70})

	// Write LUT
	lut := d.getLUT()
	d.sendCommand(cmdWriteLUT, lut)

	// Border waveform control
	d.sendCommand(cmdWriteBorder, d.getBorderSetting())

	// Set RAM address to 0, 0
	d.sendCommand(cmdSetRAMXCount, []byte{0x00})
	d.sendCommand(cmdSetRAMYCount, []byte{0x00, 0x00})

	// Write common.Black/common.White buffer
	d.sendCommand(cmdWriteRAM, d.bufferBW.Buffer())

	// Write common.Red/common.Yellow buffer
	d.sendCommand(cmdWriteAltRAM, d.bufferColor.Buffer())

	// Wait for display to be ready before activation
	if !common.BusyWait(d.config.BUSY, 5) {
		return fmt.Errorf("timeout waiting before display activation")
	}

	// Master Activate
	d.sendCommand(cmdMasterActivate, nil)

	return nil
}

// convertFramebufferToDualBuffers converts the 4-bit framebuffer to dual 1-bit buffers
// Applies rotation and offset for the SSD1608 display
// bufferBW: 0=black pixel, 1=white pixel
// bufferColor: 0=no color, 1=red/yellow pixel
func (d *SSD1608Display) convertFramebufferToDualBuffers() {
	// Clear buffers first
	bufferBW := d.bufferBW.Buffer()
	bufferColor := d.bufferColor.Buffer()
	for i := range bufferBW {
		bufferBW[i] = 0
		bufferColor[i] = 0
	}

	// Apply rotation: framebuffer is 250x122, hardware expects 136x250 (rotated -90°)
	// Also apply offset (0, 6) to account for physical display memory layout
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			color := d.GetPixel(x, y)

			// Rotate -90°: (x, y) -> (y, width-1-x)
			// Then apply offset
			rotX := y + Overscan
			rotY := Width - 1 - x

			// Skip if outside physical display bounds
			if rotX >= SSD1608Cols || rotY >= SSD1608Rows {
				continue
			}

			// Set bufferBW: 0 for black, 1 for everything else
			if color == common.Black {
				d.bufferBW.SetPixel(rotX, rotY, 0)
			} else {
				d.bufferBW.SetPixel(rotX, rotY, 1)
			}

			// Set bufferColor: 1 for red/yellow, 0 for everything else
			if color == common.Red || color == common.Yellow {
				d.bufferColor.SetPixel(rotX, rotY, 1)
			} else {
				d.bufferColor.SetPixel(rotX, rotY, 0)
			}
		}
	}
}

// getBorderSetting returns the border waveform control byte for SSD1608
func (d *SSD1608Display) getBorderSetting() []byte {
	switch d.borderColor {
	case common.Black:
		return []byte{0b00000000} // GS Transition + Waveform 00 + GSA 0 + GSB 0
	case common.Red:
		if d.colorType == "red" {
			return []byte{0b00000110} // GS Transition + Waveform 01 + GSA 1 + GSB 0
		}
	case common.Yellow:
		if d.colorType == "yellow" {
			return []byte{0b00001111} // GS Transition + Waveform 11 + GSA 1 + GSB 1
		}
	case common.White:
		return []byte{0b00000001} // GS Transition + Waveform 00 + GSA 0 + GSB 1
	}
	return []byte{0b00000000} // Default to black
}

// getLUT returns the lookup table for the SSD1608 display
// SSD1608 uses 30-byte LUT format (different from base Inky's 70-byte format)
func (d *SSD1608Display) getLUT() []byte {
	// All color variants use the same LUT for SSD1608
	return ssd1608LUT[:]
}

// Fill fills the framebuffer with a single color
func (d *SSD1608Display) Fill(color common.Color) {
	if !d.SupportsColor(color) {
		panic(fmt.Sprintf("InkySSD1608 does not support color %s (value %d)", color.String(), color))
	}

	packed := byte(color<<4) | byte(color)
	buffer := d.Buffer()
	for i := range buffer {
		buffer[i] = packed
	}
}

// SupportedColors returns the colors supported by this display
func (d *SSD1608Display) SupportedColors() []common.Color {
	if d.colorType == "red" {
		return []common.Color{common.Black, common.White, common.Red}
	} else if d.colorType == "yellow" {
		return []common.Color{common.Black, common.White, common.Yellow}
	}
	return []common.Color{common.Black, common.White, common.Red}
}

// SupportsColor checks if the display supports a specific color
func (d *SSD1608Display) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}

// SetBorder sets the border color
func (d *SSD1608Display) SetBorder(color common.Color) {
	if d.SupportsColor(color) {
		d.borderColor = color
	}
}

// LUT table for SSD1608
// 30-byte format (different from base Inky's 70-byte LUTs)
// Based on Pimoroni's Python implementation
var ssd1608LUT = [30]byte{
	0x02, 0x02, 0x01, 0x11, 0x12, 0x12, 0x22, 0x22, 0x66, 0x69,
	0x69, 0x59, 0x58, 0x99, 0x99, 0x88, 0x00, 0x00, 0x00, 0x00,
	0xF8, 0xB4, 0x13, 0x51, 0x35, 0x51, 0x51, 0x19, 0x01, 0x00,
}
