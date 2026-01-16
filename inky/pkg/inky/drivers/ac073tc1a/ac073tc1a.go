package ac073tc1a

import (
	"fmt"

	"time"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
)

const (
	Width      = 800
	Height     = 400
	ColorDepth = 4
)

// InkyAC073TC1A represents an AC073TC1A-based 7-color Inky display
// Resolution: 800x480 pixels
// Colors: common.Black, common.White, common.Green, common.Blue, common.Red, common.Yellow, common.Orange (7 colors + common.Clean mode)
type InkyAC073TC1A struct {
	config      common.InkyConfig
	borderColor common.Color
	common.Framebuffer
}

// New creates a new AC073TC1A display instance
// UNTESTED
func New(config common.InkyConfig) (*InkyAC073TC1A, error) {
	display := &InkyAC073TC1A{
		config:      config,
		borderColor: common.White,
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
func (d *InkyAC073TC1A) SupportedColors() []common.Color {
	return []common.Color{common.Black, common.White, common.Green, common.Blue, common.Red, common.Yellow, common.Orange, common.Clean}
}

// SupportsColor checks if the display supports a given color
func (d *InkyAC073TC1A) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}

// Fill fills the framebuffer with a single color
func (d *InkyAC073TC1A) Fill(color common.Color) {
	if !d.SupportsColor(color) {
		color = common.White // Default to white for unsupported colors
	}

	// Fill framebuffer with color (4-bit packed format)
	// Each byte contains 2 pixels (high nibble and low nibble)
	packed := (byte(color) << ColorDepth) | byte(color)
	fb := d.Buffer()
	for i := range fb {
		fb[i] = packed
	}
}

// SetBorderColor sets the border color for the display
func (d *InkyAC073TC1A) SetBorderColor(color common.Color) {
	if d.SupportsColor(color) {
		d.borderColor = color
	}
}

// Update transfers the framebuffer to the display and triggers refresh
// UNTESTED - No physical hardware available for testing
func (d *InkyAC073TC1A) Update() error {
	// Double hardware reset sequence
	d.config.RST.Set(false)
	time.Sleep(100 * time.Millisecond)
	d.config.RST.Set(true)
	time.Sleep(100 * time.Millisecond)
	d.config.RST.Set(false)
	time.Sleep(100 * time.Millisecond)
	d.config.RST.Set(true)

	// Wait for display to be ready after reset
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready after reset")
	}

	// Sending init commands to display
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdCMDH, []byte{0x49, 0x55, 0x20, 0x08, 0x09, 0x18})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPWR, []byte{0x3F, 0x00, 0x32, 0x2A, 0x0E, 0x2A})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPSR, []byte{0x5F, 0x69})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPOFS, []byte{0x00, 0x54, 0x00, 0x44})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdBTST1, []byte{0x40, 0x1F, 0x1F, 0x2C})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdBTST2, []byte{0x6F, 0x1F, 0x16, 0x25})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdBTST3, []byte{0x6F, 0x1F, 0x1F, 0x22})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdIPC, []byte{0x00, 0x04})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPLL, []byte{0x02})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdTSE, []byte{0x00})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdCDI, []byte{0x3F})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdTCON, []byte{0x02, 0x00})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdTRES, []byte{0x03, 0x20, 0x01, 0xE0})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdVDCS, []byte{0x1E})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdT_VDCS, []byte{0x00})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdAGID, []byte{0x00})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPWS, []byte{0x2F})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdCCSET, []byte{0x00})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdTSSET, []byte{0x00})

	// Prepare buffer: convert common.Clean (7) to common.White (1)
	fb := d.Buffer()
	var val byte
	for i := 0; i < len(fb); i++ {
		val = fb[i]
		// Check high nibble
		if (val & 0xF0) == 0x70 {
			val = (val & 0x0F) + 0x10
		}
		// Check low nibble
		if (val & 0x0F) == 0x07 {
			val = (val & 0xF0) + 0x01
		}

		fb[i] = val
	}

	// Send pixel data
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdDTM, fb)

	// Power on
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPON, nil)
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready (PON)")
	}

	// Display refresh (this takes ~45 seconds)
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdDRF, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 45) {
		return fmt.Errorf("timeout waiting for display ready (DRF)")
	}

	// Power off
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPOF, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready (POF)")
	}

	return nil
}
