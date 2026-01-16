package jd79668

import (
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/jd79661"

	"time"
)

const (
	Width      = 400
	Height     = 300
	ColorDepth = 2
)

// InkyJD79668 represents a JD79668-based 4-color Inky display
// Resolution: 400x300 pixels
// Colors: common.Black, common.White, common.Yellow, common.Red (2-bit per pixel, 4 pixels per byte)
type InkyJD79668 struct {
	config    common.InkyConfig
	colorType string // "red/yellow" for variant 24
	common.Framebuffer
}

// New creates a new JD79668 display instance
// colorType should be "red/yellow" for 4-color support
// UNTESTED
func New(config common.InkyConfig, colorType string) (*InkyJD79668, error) {
	// Validate color type
	if colorType != "red/yellow" {
		return nil, fmt.Errorf("unsupported color type '%s' for JD79668 (expected 'red/yellow')", colorType)
	}

	display := &InkyJD79668{
		config:      config,
		colorType:   colorType,
		Framebuffer: common.NewFramebuffer(Width, Height, ColorDepth),
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

	// Initial pin states
	config.CS.Set(true)
	config.DC.Set(false)
	config.RST.Set(true)

	return display, nil
}

// SupportedColors returns the colors supported by this display
func (d *InkyJD79668) SupportedColors() []common.Color {
	return []common.Color{common.Black, common.White, common.Yellow, common.Red}
}

// SupportsColor checks if the display supports a given color
func (d *InkyJD79668) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}

// Fill fills the framebuffer with a single color
func (d *InkyJD79668) Fill(color common.Color) {
	if !d.SupportsColor(color) {
		color = common.White // Default to white for unsupported colors
	}

	// Fill framebuffer with color (2-bit packed format)
	// Each byte contains 4 pixels
	packed := byte(color&0x03)<<6 | byte(color&0x03)<<4 | byte(color&0x03)<<2 | byte(color&0x03)
	buffer := d.Buffer()
	for i := range buffer {
		buffer[i] = packed
	}
}

// Update transfers the framebuffer to the display and triggers refresh
// UNTESTED - No physical hardware available for testing
func (d *InkyJD79668) Update() error {
	// Hardware reset
	common.Reset(d.config.RST)

	// Initialize display (JD79668-specific initialization)
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0x4D, []byte{0x78})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdPSR, []byte{0x0F, 0x29})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdBTST_P, []byte{0x0d, 0x12, 0x24, 0x25, 0x12, 0x29, 0x10})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0x30, []byte{0x08})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdCDI, []byte{0x37})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdTRES, []byte{
		0x01, 0x90, // X_ADDR_START: 0x0190 (400)
		0x01, 0x2C, // Y_ADDR_START: 0x012C (300)
	})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xae, []byte{0xcf})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xb0, []byte{0x13})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xbd, []byte{0x07})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xbe, []byte{0xfe})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xE9, []byte{0x01})

	// Send pixel data
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdDTM, d.Buffer())

	// Power on and refresh
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdPON, nil)
	if !common.BusyWait(d.config.BUSY, 40) {
		return fmt.Errorf("timeout waiting for display ready (PON)")
	}

	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdDRF, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 40) {
		return fmt.Errorf("timeout waiting for display ready (DRF)")
	}

	// Power off
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdPOF, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 40) {
		return fmt.Errorf("timeout waiting for display ready (POF)")
	}

	// Deep sleep
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, jd79661.CmdDSLP, []byte{0xA5})
	time.Sleep(100 * time.Millisecond)

	return nil
}
