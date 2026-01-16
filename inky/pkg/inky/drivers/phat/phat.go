package phat

import (
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
)

// Display constants for Inky pHAT (212x104)
const (
	Width      = 212
	Height     = 104
	ColorDepth = 2
)

// PHATDisplay represents an Inky pHAT 2.13" e-ink display
// This is a 3-color display supporting common.Black, common.White, and common.Red or common.Yellow
type PHATDisplay struct {
	config             common.InkyConfig
	colorType          string // "red", "yellow", or "black"
	lutType            string // LUT variant to use ("red", "red_ht", "yellow", "black")
	borderColor        common.Color
	bufferBW           common.Framebuffer // 1-bit packed: common.Black/common.White buffer for hardware
	bufferColor        common.Framebuffer // 1-bit packed: common.Red/common.Yellow buffer for hardware
	common.Framebuffer                    // combined 4 color buffer exposed to consumers
}

// New creates and initializes an Inky pHAT 2.13" display (212x104)
// colorType should be "red", "yellow", or "black" to match the physical display
// UNTESTED: This implementation has not been tested on physical hardware
func New(config common.InkyConfig, colorType string) (*PHATDisplay, error) {
	// Validate color type
	if colorType != "red" && colorType != "yellow" && colorType != "black" {
		return nil, fmt.Errorf("invalid color type %q, must be 'red', 'yellow', or 'black'", colorType)
	}

	display := &PHATDisplay{
		config:      config,
		colorType:   colorType,
		lutType:     colorType,
		borderColor: common.Black,
		Framebuffer: common.NewFramebuffer(Width, Height, ColorDepth),
		// Physical display is rotated 90deg
		bufferBW:    common.NewFramebuffer(Height, Width, ColorDepth/2),
		bufferColor: common.NewFramebuffer(Height, Width, ColorDepth/2),
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

	// Check if we should use high-temperature LUT variant
	// This requires EEPROM check, but for now we'll use standard LUTs
	// TODO: Read EEPROM to detect variant 1 or 6 with red for "red_ht" LUT

	// Perform hardware initialization
	if err := display.init(); err != nil {
		return nil, fmt.Errorf("display initialization failed: %w", err)
	}

	return display, nil
}

// init initializes the display controller
// Based on Pimoroni's Python base Inky class implementation
func (d *PHATDisplay) init() error {
	// Hardware reset
	common.Reset(d.config.RST)

	// Send soft reset command
	d.sendCommand(CmdDRF, nil)

	// Wait for display to be ready (1 second timeout)
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready after reset")
	}

	return nil
}

// sendCommand sends a command with optional data to the display
func (d *PHATDisplay) sendCommand(command byte, data []byte) {
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, command, data)
}

// Update transfers the framebuffer to the display and triggers a refresh
func (d *PHATDisplay) Update() error {
	// Convert 4-bit framebuffer to dual 1-bit buffers
	d.convertFramebufferToDualBuffers()

	// Setup display for update
	// Based on Python _update() method

	// Set Analog Block Control
	d.sendCommand(CmdABC, []byte{0x54})

	// Set Digital Block Control
	d.sendCommand(CmdDBC, []byte{0x3B})

	// Gate setting (rows as little-endian uint16)
	rowsLow := byte(Width & 0xFF)
	rowsHigh := byte((Width >> 8) & 0xFF)
	d.sendCommand(CmdPWR, []byte{rowsHigh, rowsLow, 0x00})

	// Gate Driving Voltage
	d.sendCommand(CmdPOFS, []byte{0x17})

	// Source Driving Voltage
	d.sendCommand(CmdPON, []byte{0x41, 0xAC, 0x32})

	// Dummy line period
	d.sendCommand(CmdDLP, []byte{0x07})

	// Gate line width
	d.sendCommand(CmdGLW, []byte{0x04})

	// Data entry mode setting (0x03 = X/Y increment)
	d.sendCommand(CmdDSP, []byte{0x03})

	// VCOM Register
	d.sendCommand(CmdVCOM, []byte{0x3C})

	// Border waveform control
	d.sendCommand(CmdBDR, d.getBorderSetting())

	// Adjust voltage settings for yellow displays
	if d.colorType == "yellow" {
		d.sendCommand(CmdPON, []byte{0x07, 0xAC, 0x32})
	}

	// Write LUT
	lut := GetLUT(d.lutType)
	d.sendCommand(CmdWLUT, lut)

	// Set RAM X Start/End (cols / 8 - 1)
	d.sendCommand(CmdRAMXS, []byte{0x00, byte(Height/8) - 1})

	// Set RAM Y Start/End
	d.sendCommand(CmdRAMYS, []byte{0x00, 0x00, rowsHigh, rowsLow})

	// Write common.Black/common.White buffer
	d.sendCommand(CmdRAMXP, []byte{0x00})       // Set RAM X Pointer
	d.sendCommand(CmdRAMYP, []byte{0x00, 0x00}) // Set RAM Y Pointer
	d.sendCommand(CmdRAMBW, d.bufferBW.Buffer())

	// Write common.Red/common.Yellow buffer
	d.sendCommand(CmdRAMXP, []byte{0x00})       // Set RAM X Pointer
	d.sendCommand(CmdRAMYP, []byte{0x00, 0x00}) // Set RAM Y Pointer
	d.sendCommand(CmdRAMRY, d.bufferColor.Buffer())

	// Display Update Sequence
	d.sendCommand(CmdUPDSQ, []byte{0xC7})

	// Trigger common.Display Update
	d.sendCommand(CmdTRIGR, nil)

	// Wait for update to complete (30 second timeout)
	if !common.BusyWait(d.config.BUSY, 30) {
		return fmt.Errorf("timeout waiting for display update")
	}

	// Enter Deep Sleep
	d.sendCommand(CmdDTM1, []byte{0x01})

	return nil
}

// convertFramebufferToDualBuffers converts the 4-bit framebuffer to dual 1-bit buffers
// bufferBW: 0=black pixel, 1=white pixel
// bufferColor: 0=no color, 1=red/yellow pixel
func (d *PHATDisplay) convertFramebufferToDualBuffers() {
	// Apply rotation: framebuffer is 212x104, but hardware expects 104x212 (rotated -90°)
	// We need to rotate the image by -90 degrees (or +270 degrees)

	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			color := d.GetPixel(x, y)

			// Rotate -90°: (x, y) -> (y, width-1-x)
			// New coordinates in rotated space
			rotX := y
			rotY := Width - 1 - x

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

// getBorderSetting returns the border waveform control byte
func (d *PHATDisplay) getBorderSetting() []byte {
	switch d.borderColor {
	case common.Black:
		return []byte{0b00000000} // GS Transition Define A + VSS + LUT0
	case common.Red:
		if d.colorType == "red" {
			return []byte{0b01110011} // Fix Level Define A + VSH2 + LUT3
		}
	case common.Yellow:
		if d.colorType == "yellow" {
			return []byte{0b00110011} // GS Transition Define A + VSH2 + LUT3
		}
	case common.White:
		return []byte{0b00110001} // GS Transition Define A + VSH2 + LUT1
	}
	return []byte{0b00000000} // Default to black
}

// Fill fills the framebuffer with a single color
func (d *PHATDisplay) Fill(color common.Color) {
	if !d.SupportsColor(color) {
		panic(fmt.Sprintf("InkyPHAT does not support color %s (value %d)", color.String(), color))
	}

	packed := byte(color<<4) | byte(color)
	buffer := d.Buffer()
	for i := range buffer {
		buffer[i] = packed
	}
}

// SupportedColors returns the colors supported by this display
func (d *PHATDisplay) SupportedColors() []common.Color {
	if d.colorType == "red" {
		return []common.Color{common.Black, common.White, common.Red}
	} else if d.colorType == "yellow" {
		return []common.Color{common.Black, common.White, common.Yellow}
	}
	// "black" variant still supports all colors, just no chromatic display
	return []common.Color{common.Black, common.White, common.Red}
}

// SupportsColor checks if the display supports a specific color
func (d *PHATDisplay) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}

// SetBorder sets the border color
func (d *PHATDisplay) SetBorder(color common.Color) {
	if d.SupportsColor(color) {
		d.borderColor = color
	}
}
