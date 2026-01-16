package e673

import (
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/phat"
)

// Display constants for Inky Impression 7.3" Spectra 6 (E673)
const (
	Width      = 800
	Height     = 480
	ColorDepth = 4
)

// E673Display represents an Inky E673 7.3" Spectra 6 e-ink display
type E673Display struct {
	config common.InkyConfig
	common.Framebuffer
}

// New creates and initializes an Inky Impression 7.3" Spectra 6 display (E673)
func New(config common.InkyConfig) (*E673Display, error) {
	display := &E673Display{
		config:      config,
		Framebuffer: common.NewFramebuffer(Width, Height, ColorDepth),
	}

	// Configure pins for this display
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
	config.CS.Set(true)  // CS idle high
	config.DC.Set(false) // DC default low
	config.RST.Set(true) // RST idle high

	// Perform hardware initialization
	if err := display.init(); err != nil {
		return nil, fmt.Errorf("display initialization failed: %w", err)
	}

	return display, nil
}

// init initializes the e673 controller
// Based on Pimoroni's Python implementation
func (d *E673Display) init() error {
	// Hardware reset
	common.Reset(d.config.RST)

	// Wait for display to be ready
	if !common.BusyWait(d.config.BUSY, 5) {
		return fmt.Errorf("timeout waiting for display ready after reset")
	}

	// Send initialization sequence from e673.py
	// Magic initialization command
	d.sendCommand(0xAA, []byte{0x49, 0x55, 0x20, 0x08, 0x09, 0x18})

	// Power setting
	d.sendCommand(phat.CmdPWR, []byte{0x3F})

	// Panel setting register
	d.sendCommand(phat.CmdPSR, []byte{0x5F, 0x69})

	// Booster soft start settings
	d.sendCommand(phat.CmdBTST1, []byte{0x40, 0x1F, 0x1F, 0x2C})
	d.sendCommand(phat.CmdBTST3, []byte{0x6F, 0x1F, 0x1F, 0x22})
	d.sendCommand(phat.CmdBTST2, []byte{0x6F, 0x1F, 0x17, 0x17})

	// Power off sequence setting
	d.sendCommand(phat.CmdPOFS, []byte{0x00, 0x54, 0x00, 0x44})

	// TCON setting
	d.sendCommand(phat.CmdTCON, []byte{0x02, 0x00})

	// PLL control
	d.sendCommand(phat.CmdPLL, []byte{0x08})

	// VCOM and data interval setting
	d.sendCommand(phat.CmdCDI, []byte{0x3F})

	// Resolution setting (800x480)
	// 0x0320 = 800, 0x01E0 = 480
	d.sendCommand(phat.CmdTRES, []byte{0x03, 0x20, 0x01, 0xE0})

	// Power saving
	d.sendCommand(phat.CmdPWS, []byte{0x2F})

	// VCOM DC setting
	d.sendCommand(phat.CmdVDCS, []byte{0x01})

	return nil
}

// sendCommand sends a command with optional data to the display
func (d *E673Display) sendCommand(command byte, data []byte) {
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, command, data)
}

// Update transfers the framebuffer to the display and triggers a refresh
// This takes approximately 32 seconds to complete for the E673
func (d *E673Display) Update() error {
	// Power on
	d.sendCommand(phat.CmdPON, nil)
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for power on")
	}

	// Send packed image data to display
	d.sendCommand(phat.CmdDTM1, d.Buffer())

	// Second setting of BTST2 register (from Python implementation)
	// Note: E673 uses 0x49 instead of E640's 0x47
	d.sendCommand(phat.CmdBTST2, []byte{0x6F, 0x1F, 0x17, 0x49})

	// Display refresh
	d.sendCommand(phat.CmdDRF, []byte{0x00})

	// Wait for refresh to complete (E673 takes ~32 seconds)
	println("Refreshing display (this takes ~32 seconds)...")
	if !common.BusyWait(d.config.BUSY, 35) {
		return fmt.Errorf("timeout waiting for display refresh")
	}

	// Power off
	d.sendCommand(phat.CmdPOF, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for power off")
	}

	return nil
}

// Fill fills the framebuffer with a single color
func (d *E673Display) Fill(color common.Color) {
	// Validate color is supported
	if !d.SupportsColor(color) {
		panic(fmt.Sprintf("E673 does not support color %s (value %d)", color.String(), color))
	}

	// Pack the color: 2 pixels per byte
	packed := byte(color<<4) | byte(color)
	fb := d.Buffer()
	for i := range fb {
		fb[i] = packed
	}
}

// SupportedColors returns the list of colors supported by this display
func (d *E673Display) SupportedColors() []common.Color {
	return []common.Color{common.Black, common.White, common.Yellow, common.Red, common.Blue, common.Green}
}

// SupportsColor checks if the display supports a specific color
func (d *E673Display) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}
