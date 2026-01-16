package uc8159

import (
	"encoding/binary"
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"

	"time"
)

const ColorDepth = 4

// InkyUC8159 represents a UC8159-based 7-color Inky display
// Resolutions: 600x448 (variant 14) or 640x400 (variants 15, 16)
// Colors: common.Black, common.White, common.Green, common.Blue, common.Red, common.Yellow, common.Orange (7 colors + common.Clean mode)
type InkyUC8159 struct {
	config            common.InkyConfig
	resolutionSetting byte // 0b11 for 600x448, 0b10 for 640x400
	borderColor       common.Color
	width, height     int
	common.Framebuffer
}

// New creates a new UC8159 display instance
// Supports two resolutions:
// - 600x448 (variant 14): resolutionSetting = 0b11
// - 640x400 (variants 15, 16): resolutionSetting = 0b10
// UNTESTED
func New(config common.InkyConfig, width, height int) (*InkyUC8159, error) {
	// Validate resolution and set resolution setting
	var resolutionSetting byte
	switch {
	case width == 600 && height == 448:
		resolutionSetting = 0b11
	case width == 640 && height == 400:
		resolutionSetting = 0b10
	default:
		return nil, fmt.Errorf("unsupported resolution %dx%d for UC8159 (expected 600x448 or 640x400)", width, height)
	}

	display := &InkyUC8159{
		config:            config,
		width:             width,
		height:            height,
		resolutionSetting: resolutionSetting,
		borderColor:       common.White,
		Framebuffer:       common.NewFramebuffer(width, height, ColorDepth),
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
func (d *InkyUC8159) SupportedColors() []common.Color {
	return []common.Color{common.Black, common.White, common.Green, common.Blue, common.Red, common.Yellow, common.Orange, common.Clean}
}

// SupportsColor checks if the display supports a given color
func (d *InkyUC8159) SupportsColor(color common.Color) bool {
	for _, c := range d.SupportedColors() {
		if c == color {
			return true
		}
	}
	return false
}

// Fill fills the framebuffer with a single color
func (d *InkyUC8159) Fill(color common.Color) {
	if !d.SupportsColor(color) {
		color = common.White // Default to white for unsupported colors
	}

	// Fill framebuffer with color (4-bit packed format)
	// Each byte contains 2 pixels (high nibble and low nibble)
	packed := (byte(color) << 4) | byte(color)
	buffer := d.Buffer()
	for i := range buffer {
		buffer[i] = packed
	}
}

// SetBorderColor sets the border color for the display
func (d *InkyUC8159) SetBorderColor(color common.Color) {
	if d.SupportsColor(color) {
		d.borderColor = color
	}
}

// Update transfers the framebuffer to the display and triggers refresh
func (d *InkyUC8159) Update() error {
	// Hardware reset
	d.config.RST.Set(false)
	time.Sleep(100 * time.Millisecond)
	d.config.RST.Set(true)
	time.Sleep(100 * time.Millisecond)

	// Wait for display to be ready after reset
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready after reset")
	}

	// Resolution Setting
	// 10-bit horizontal followed by 10-bit vertical resolution
	// Using big-endian 16-bit values
	resData := make([]byte, 4)
	binary.BigEndian.PutUint16(resData[0:2], uint16(d.width))
	binary.BigEndian.PutUint16(resData[2:4], uint16(d.height))
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdTRES, resData)

	// Panel Setting Register
	// Bit 7-6: Resolution select (0b11 = 600x448, 0b10 = 640x400)
	// Bit 5: LUT selection (0 = ext flash, 1 = registers)
	// Bit 4: Ignore
	// Bit 3: Gate scan direction (0 = down, 1 = up)
	// Bit 2: Source shift direction (0 = left, 1 = right)
	// Bit 1: DC-DC converter (0 = off, 1 = on)
	// Bit 0: Soft reset (0 = reset, 1 = normal)
	psrByte1 := (d.resolutionSetting << 6) | 0b101111
	psrByte2 := byte(0x08) // UC8159_7C (7-color mode)
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPSR, []byte{psrByte1, psrByte2})

	// Power Settings
	// Bit 5-3: ??? (not documented)
	// Bit 2: SOURCE_INTERNAL_DC_DC
	// Bit 1: GATE_INTERNAL_DC_DC
	// Bit 0: LV_SOURCE_INTERNAL_DC_DC
	pwrData := []byte{
		(0x06 << 3) | (0x01 << 2) | (0x01 << 1) | 0x01,
		0x00, // VGx_20V
		0x23, // UC8159_7C
		0x23, // UC8159_7C
	}
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPWR, pwrData)

	// Set PLL clock frequency
	// PLL = 2MHz * (M / N)
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPLL, []byte{0x3C})

	// Temperature Sensor Enable (color mode)
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdTSE, []byte{0x00})

	// VCOM and Data Interval Setting
	// Bit 7-5: Vborder control (border color)
	// Bit 4: Data polarity
	// Bit 3-0: VCOM and data interval
	cdi := (byte(d.borderColor) << 5) | 0x17
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdCDI, []byte{cdi})

	// TCON Setting (Gate/Source non-overlap period)
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdTCON, []byte{0x22})

	// Disable external flash
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdDAM, []byte{0x00})

	// Power Saving (UC8159_7C specific)
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPWS, []byte{0xAA})

	// Power Off Sequence Setting
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPFS, []byte{0x00})

	// Send pixel data
	// Framebuffer is already in the correct format (4-bit packed, 2 pixels per byte)
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdDTM1, d.Buffer())

	// Power on
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPON, nil)
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready (PON)")
	}

	// Display refresh
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdDRF, nil)
	if !common.BusyWait(d.config.BUSY, 32) {
		return fmt.Errorf("timeout waiting for display ready (DRF)")
	}

	// Power off
	common.SendCommand(d.config.SPI, d.config.CS, d.config.DC, cmdPOF, nil)
	if !common.BusyWait(d.config.BUSY, 1) {
		return fmt.Errorf("timeout waiting for display ready (POF)")
	}

	return nil
}
