package what

import (
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/phat"
)

// Display constants for Inky wHAT (400x300)
const (
	Width      = 400
	Height     = 300
	ColorDepth = 2
)

// WHATDisplay represents an Inky wHAT 4.2" e-ink display
// This is a 3-color display supporting common.Black, common.White, and common.Red or common.Yellow
type WHATDisplay struct {
	config             common.InkyConfig
	colorType          string // "red", "yellow", or "black"
	lutType            string // LUT variant to use ("red", "red_ht", "yellow", "black")
	borderColor        common.Color
	bufferBW           common.Framebuffer // 1-bit packed: common.Black/common.White buffer for hardware
	bufferColor        common.Framebuffer // 1-bit packed: common.Red/common.Yellow buffer for hardware
	common.Framebuffer                    // combined 4 color buffer exposed to consumers
}

// New creates and initializes an Inky wHAT 4.2" display (400x300)
// colorType should be "red", "yellow", or "black" to match the physical display
// UNTESTED: This implementation has not been tested on physical hardware
func New(config common.InkyConfig, colorType string) (*WHATDisplay, error) {
	// Validate color type
	if colorType != "red" && colorType != "yellow" && colorType != "black" {
		return nil, fmt.Errorf("invalid color type %q, must be 'red', 'yellow', or 'black'", colorType)
	}

	display := &WHATDisplay{
		config:      config,
		colorType:   colorType,
		lutType:     colorType,
		borderColor: common.Black,
		Framebuffer: common.NewFramebuffer(Width, Height, ColorDepth),
		bufferBW:    common.NewFramebuffer(Width, Height, ColorDepth/2),
		bufferColor: common.NewFramebuffer(Width, Height, ColorDepth/2),
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

// init initializes the display controller
// Based on Pimoroni's Python base Inky class implementation
func (d *WHATDisplay) init() error {
	// Hardware reset
	common.Reset(d.config.RST)

	// Send soft reset command
	d.sendCommand(phat.CmdDRF, nil)

	// Wait for display to be ready (1 second timeout)
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready after reset")
	}

	return nil
}

// sendCommand sends a command with optional data to the display
func (d *WHATDisplay) sendCommand(command byte, data []byte) {
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, command, data)
}

// Update transfers the framebuffer to the display and triggers a refresh
// Converts the 4-bit framebuffer to dual 1-bit buffers for the hardware
// UNTESTED: This implementation has not been tested on physical hardware
func (d *WHATDisplay) Update() error {
	// Convert 4-bit framebuffer to dual 1-bit buffers
	d.convertFramebufferToDualBuffers()

	// Setup display for update
	// Based on Python _update() method

	// Set Analog Block Control
	d.sendCommand(phat.CmdABC, []byte{0x54})

	// Set Digital Block Control
	d.sendCommand(phat.CmdDBC, []byte{0x3B})

	// Gate setting (rows as little-endian uint16)
	rowsLow := byte(Height & 0xFF)
	rowsHigh := byte((Height >> 8) & 0xFF)
	d.sendCommand(phat.CmdPWR, []byte{rowsHigh, rowsLow, 0x00})

	// Gate Driving Voltage
	d.sendCommand(phat.CmdPOFS, []byte{0x17})

	// Source Driving Voltage
	d.sendCommand(phat.CmdPON, []byte{0x41, 0xAC, 0x32})

	// Dummy line period
	d.sendCommand(phat.CmdDLP, []byte{0x07})

	// Gate line width
	d.sendCommand(phat.CmdGLW, []byte{0x04})

	// Data entry mode setting (0x03 = X/Y increment)
	d.sendCommand(phat.CmdDSP, []byte{0x03})

	// VCOM Register
	d.sendCommand(phat.CmdVCOM, []byte{0x3C})

	// Border waveform control
	d.sendCommand(phat.CmdBDR, d.getBorderSetting())

	// Adjust voltage settings for yellow displays
	if d.colorType == "yellow" {
		d.sendCommand(phat.CmdPON, []byte{0x07, 0xAC, 0x32})
	}

	// Adjust voltage for 400x300 red displays
	if d.colorType == "red" {
		d.sendCommand(phat.CmdPON, []byte{0x30, 0xAC, 0x22})
	}

	// Write LUT
	lut := phat.GetLUT(d.lutType)
	d.sendCommand(phat.CmdWLUT, lut)

	// Set RAM X Start/End (cols / 8 - 1)
	d.sendCommand(phat.CmdRAMXS, []byte{0x00, byte(Width/8) - 1})

	// Set RAM Y Start/End
	d.sendCommand(phat.CmdRAMYS, []byte{0x00, 0x00, rowsHigh, rowsLow})

	// Write common.Black/common.White buffer
	d.sendCommand(phat.CmdRAMXP, []byte{0x00})       // Set RAM X Pointer
	d.sendCommand(phat.CmdRAMYP, []byte{0x00, 0x00}) // Set RAM Y Pointer
	d.sendCommand(phat.CmdRAMBW, d.bufferBW.Buffer())

	// Write common.Red/common.Yellow buffer
	d.sendCommand(phat.CmdRAMXP, []byte{0x00})       // Set RAM X Pointer
	d.sendCommand(phat.CmdRAMYP, []byte{0x00, 0x00}) // Set RAM Y Pointer
	d.sendCommand(phat.CmdRAMRY, d.bufferColor.Buffer())

	// Display Update Sequence
	d.sendCommand(phat.CmdUPDSQ, []byte{0xC7})

	// Trigger common.Display Update
	d.sendCommand(phat.CmdTRIGR, nil)

	// Wait for update to complete (30 second timeout)
	if !common.BusyWait(d.config.BUSY, 30) {
		return fmt.Errorf("timeout waiting for display update")
	}

	// Enter Deep Sleep
	d.sendCommand(phat.CmdDTM1, []byte{0x01})

	return nil
}

// convertFramebufferToDualBuffers converts the 4-bit framebuffer to dual 1-bit buffers
// bufferBW: 0=black pixel, 1=white pixel
// bufferColor: 0=no color, 1=red/yellow pixel
func (d *WHATDisplay) convertFramebufferToDualBuffers() {
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			color := d.GetPixel(x, y)

			// Set bufferBW: 0 for black, 1 for everything else
			if color == common.Black {
				d.bufferBW.SetPixel(x, y, 0)
			} else {
				d.bufferBW.SetPixel(x, y, 1)
			}

			// Set bufferColor: 1 for red/yellow, 0 for everything else
			if color == common.Red || color == common.Yellow {
				d.bufferColor.SetPixel(x, y, 1)
			} else {
				d.bufferColor.SetPixel(x, y, 0)
			}
		}
	}
}

// getBorderSetting returns the border waveform control byte
func (d *WHATDisplay) getBorderSetting() []byte {
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
func (d *WHATDisplay) Fill(color common.Color) {
	if !d.SupportsColor(color) {
		panic(fmt.Sprintf("InkyWHAT does not support color %s (value %d)", color.String(), color))
	}

	packed := byte(color<<4) | byte(color)
	buffer := d.Buffer()
	for i := range buffer {
		buffer[i] = packed
	}
}

// SupportedColors returns the colors supported by this display
func (d *WHATDisplay) SupportedColors() []common.Color {
	if d.colorType == "red" {
		return []common.Color{common.Black, common.White, common.Red}
	} else if d.colorType == "yellow" {
		return []common.Color{common.Black, common.White, common.Yellow}
	}
	// "black" variant still supports all colors, just no chromatic display
	return []common.Color{common.Black, common.White, common.Red}
}

// SupportsColor checks if the display supports a specific color
func (d *WHATDisplay) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}

// SetBorder sets the border color
func (d *WHATDisplay) SetBorder(color common.Color) {
	if d.SupportsColor(color) {
		d.borderColor = color
	}
}
