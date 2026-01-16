package jd79661

import (
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"

	"time"
)

const (
	Width      = 250
	Height     = 122
	ColorDepth = 2
	Overscan   = 6
)

// InkyJD79661 represents a JD79661-based 4-color Inky display
// Resolution: 250x122 pixels
// Colors: common.Black, common.White, common.Yellow, common.Red (2-bit per pixel, 4 pixels per byte)
type InkyJD79661 struct {
	config    common.InkyConfig
	colorType string // "red/yellow" for variant 23
	intBuffer common.Framebuffer
	common.Framebuffer
}

// New creates a new JD79661 display instance
// colorType should be "red/yellow" for 4-color support
// UNTESTED
func New(config common.InkyConfig, colorType string) (*InkyJD79661, error) {
	// Validate color type
	if colorType != "red/yellow" {
		return nil, fmt.Errorf("unsupported color type '%s' for JD79661 (expected 'red/yellow')", colorType)
	}

	display := &InkyJD79661{
		config:      config,
		colorType:   colorType,
		Framebuffer: common.NewFramebuffer(Width, Height, ColorDepth),
		// Internal buffer to handle rotation and different resolution of physical display
		intBuffer: common.NewFramebuffer(Height+Overscan, Width, ColorDepth),
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
func (d *InkyJD79661) SupportedColors() []common.Color {
	return []common.Color{common.Black, common.White, common.Yellow, common.Red}
}

// SupportsColor checks if the display supports a given color
func (d *InkyJD79661) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}

// Fill fills the framebuffer with a single color
func (d *InkyJD79661) Fill(color common.Color) {
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
func (d *InkyJD79661) Update() error {
	// Hardware reset
	common.Reset(d.config.RST)

	// Initialize display
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0x4D, []byte{0x78})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdPSR, []byte{0x0F, 0x29})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdPWR, []byte{0x07, 0x00})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdPOFS, []byte{0x10, 0x54, 0x44})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdBTST_P, []byte{0x0F, 0x0A, 0x2F, 0x25, 0x22, 0x2E, 0x21})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdCDI, []byte{0x37})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdTCON, []byte{0x02, 0x02})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdTRES, []byte{
		jd79661_X_ADDR_START_H, jd79661_X_ADDR_START_L,
		jd79661_Y_ADDR_START_H, jd79661_Y_ADDR_START_L,
	})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xE7, []byte{0x1C})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdPWS, []byte{0x22})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xB6, []byte{0x6F})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xB4, []byte{0xD0})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0xE9, []byte{0x01})
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, 0x30, []byte{0x08})

	// Prepare display buffer with rotation and packing
	d.prepareBuffer()

	// Send pixel data
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdDTM, d.Buffer())

	// Power on and refresh
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdPON, nil)
	if !common.BusyWait(d.config.BUSY, 40) {
		return fmt.Errorf("timeout waiting for display ready (PON)")
	}

	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdDRF, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 40) {
		return fmt.Errorf("timeout waiting for display ready (DRF)")
	}

	// Power off
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdPOF, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 40) {
		return fmt.Errorf("timeout waiting for display ready (POF)")
	}

	// Deep sleep
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, CmdDSLP, []byte{0xA5})
	time.Sleep(100 * time.Millisecond)

	return nil
}

// prepareBuffer converts framebuffer to display format with rotation
func (d *InkyJD79661) prepareBuffer() {
	intBuffer := d.intBuffer.Buffer()

	// zero out buffer
	for i := range intBuffer {
		intBuffer[i] = 0
	}

	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			color := d.GetPixel(x, y)

			rotX := y + Overscan
			rotY := Width - 1 - x

			d.intBuffer.SetPixel(rotX, rotY, color)
		}
	}
}
