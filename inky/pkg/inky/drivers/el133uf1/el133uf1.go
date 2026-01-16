package el133uf1

import (
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"

	"time"
)

const (
	Width      = 1600
	Height     = 1200
	ColorDepth = 4
)

// InkyEL133UF1 represents an EL133UF1-based 6-color Inky display
// Resolution: 1600x1200 pixels
// Colors: common.Black, common.White, common.Yellow, common.Red, common.Blue, common.Green (6 colors, no common.Orange or common.Clean)
type InkyEL133UF1 struct {
	config             common.InkyConfig
	borderColor        common.Color
	bufferH1, bufferH2 common.Framebuffer
	common.Framebuffer
}

// New creates a new EL133UF1 display instance
// NOTE: Requires config.CS1 to be set (dual chip select display)
// WARNING: This display is large and requires double buffering due to rotation.
// Its framebuffer won't fit into the memory of most common microcontrollers
// UNTESTED
func New(config common.InkyConfig) (*InkyEL133UF1, error) {
	// Validate that CS1 is provided (required for dual-CS display)
	if config.CS1 == nil {
		return nil, fmt.Errorf("EL133UF1 requires CS1 pin (dual chip select display)")
	}

	display := &InkyEL133UF1{
		config:      config,
		borderColor: common.White,
		// Internal buffers, rotated and split into two halfs
		bufferH1:    common.NewFramebuffer(Width, Height/2, ColorDepth),
		bufferH2:    common.NewFramebuffer(Width, Height/2, ColorDepth),
		Framebuffer: common.NewFramebuffer(Width, Height, ColorDepth),
	}

	// Configure pins
	if err := config.CS.Configure(common.PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure CS pin: %w", err)
	}
	if err := config.CS1.Configure(common.PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure CS1 pin: %w", err)
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
	config.CS1.Set(true)
	config.DC.Set(false)
	config.RST.Set(true)

	return display, nil
}

// SupportedColors returns the colors supported by this display
func (d *InkyEL133UF1) SupportedColors() []common.Color {
	return []common.Color{common.Black, common.White, common.Yellow, common.Red, common.Blue, common.Green}
}

// SupportsColor checks if the display supports a given color
func (d *InkyEL133UF1) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}

// Fill fills the framebuffer with a single color
func (d *InkyEL133UF1) Fill(color common.Color) {
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
func (d *InkyEL133UF1) SetBorderColor(color common.Color) {
	if d.SupportsColor(color) {
		d.borderColor = color
	}
}

// sendCommand sends a command to selected chip select pins
func (d *InkyEL133UF1) sendCommand(command byte, csSelect byte, data []byte) {
	// Set chip select pins based on selection
	if csSelect&cs0Sel != 0 {
		d.config.CS.Set(false)
	}
	if csSelect&cs1Sel != 0 {
		d.config.CS1.Set(false)
	}

	d.config.DC.Set(false)
	time.Sleep(300 * time.Microsecond)
	d.config.SPI.Tx([]byte{command}, nil)

	if data != nil && len(data) > 0 {
		d.config.DC.Set(true)
		d.config.SPI.Tx(data, nil)
	}

	// Deassert all chip selects
	d.config.CS.Set(true)
	d.config.CS1.Set(true)
	d.config.DC.Set(false)
}

// Update transfers the framebuffer to the display and triggers refresh
// UNTESTED - No physical hardware available for testing
func (d *InkyEL133UF1) Update() error {
	// Hardware reset
	d.config.RST.Set(false)
	time.Sleep(30 * time.Millisecond)
	d.config.RST.Set(true)
	time.Sleep(30 * time.Millisecond)

	// Wait for display to be ready after reset
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready after reset")
	}

	// Initialization sequence
	d.sendCommand(cmdANTM, cs0Sel, []byte{0xC0, 0x1C, 0x1C, 0xCC, 0xCC, 0xCC, 0x15, 0x15, 0x55})
	d.sendCommand(cmdCMD66, csBoth, []byte{0x49, 0x55, 0x13, 0x5D, 0x05, 0x10})
	d.sendCommand(cmdPSR, csBoth, []byte{0xDF, 0x69})
	d.sendCommand(cmdPLL, csBoth, []byte{0x08})
	d.sendCommand(cmdCDI, csBoth, []byte{0xF7})
	d.sendCommand(cmdTCON, csBoth, []byte{0x03, 0x03})
	d.sendCommand(cmdAGID, csBoth, []byte{0x10})
	d.sendCommand(cmdPWS, csBoth, []byte{0x22})
	d.sendCommand(cmdCCSET, csBoth, []byte{0x01})
	d.sendCommand(cmdTRES, csBoth, []byte{0x04, 0xB0, 0x03, 0x20})

	// CS0-specific initialization
	d.sendCommand(cmdPWR, cs0Sel, []byte{0x0F, 0x00, 0x28, 0x2C, 0x28, 0x38})
	d.sendCommand(cmdEN_BUF, cs0Sel, []byte{0x07})
	d.sendCommand(cmdBTST_P, cs0Sel, []byte{0xD8, 0x18})
	d.sendCommand(cmdBOOST_VDDP_EN, cs0Sel, []byte{0x01})
	d.sendCommand(cmdBTST_N, cs0Sel, []byte{0xD8, 0x18})
	d.sendCommand(cmdBUCK_BOOST_VDDN, cs0Sel, []byte{0x01})
	d.sendCommand(cmdTFT_VCOM_POWER, cs0Sel, []byte{0x02})

	d.prepareBuffers()

	// Send pixel data to each CS
	d.sendCommand(cmdDTM, cs0Sel, d.bufferH1.Buffer())
	d.sendCommand(cmdDTM, cs1Sel, d.bufferH2.Buffer())

	// Power on
	d.sendCommand(cmdPON, csBoth, nil)
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready (PON)")
	}

	// Display refresh
	d.sendCommand(cmdDRF, csBoth, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 32) {
		return fmt.Errorf("timeout waiting for display ready (DRF)")
	}

	// Power off
	d.sendCommand(cmdPOF, csBoth, []byte{0x00})
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready (POF)")
	}

	return nil
}

// prepareBuffers applies rotation, color remapping, and splits into two halves
func (d *InkyEL133UF1) prepareBuffers() {

	// Remap colors (sequential [0,1,2,3,4,5] → display [0,1,2,3,5,6])
	colorRemap := []common.Color{0, 1, 2, 3, 5, 6}

	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			color := d.GetPixel(x, y)

			// The display doesn't support color 4, so we need to remap
			color = colorRemap[color]

			// Rotate -90°: (x, y) -> (y, width-1-x)
			// New coordinates in rotated space
			rotX := y
			rotY := Width - 1 - x

			// This is after rotation so we actually use halfHeight
			halfWidth := Height / 2

			if rotX < halfWidth {
				d.bufferH1.SetPixel(rotX, rotY, color)
			} else {
				d.bufferH2.SetPixel(rotX-halfWidth, rotY, color)
			}
		}
	}
}
