package inky

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
)

// EEPROM constants
const (
	eepromAddress = 0x50
	eepromSize    = 29
)

// Display variant names mapping (from Pimoroni's Python implementation)
var displayVariantNames = map[byte]string{
	1:  "Red pHAT (High-Temp)",
	2:  "Yellow wHAT",
	3:  "Black wHAT",
	4:  "Black pHAT",
	5:  "Yellow pHAT",
	6:  "Red wHAT",
	7:  "Red wHAT (High-Temp)",
	8:  "Red wHAT",
	10: "Black pHAT (SSD1608)",
	11: "Red pHAT (SSD1608)",
	12: "Yellow pHAT (SSD1608)",
	14: "7-Colour (UC8159)",
	15: "7-Colour 640x400 (UC8159)",
	16: "7-Colour 640x400 (UC8159)",
	17: "Black wHAT (SSD1683)",
	18: "Red wHAT (SSD1683)",
	19: "Yellow wHAT (SSD1683)",
	20: "7-Colour 800x480 (AC073TC1A)",
	21: "Spectra 6 13.3 1600 x 1200 (EL133UF1)",
	22: "Spectra 6 7.3 800 x 480 (E673)",
	23: "Red/Yellow pHAT (JD79661)",
	24: "Red/Yellow wHAT (JD79668)",
	25: "Spectra 6 4.0 400 x 600 (E640)",
}

// EEPROMData represents the parsed EEPROM structure from an Inky display
type EEPROMData struct {
	Width          uint16 // Display width in pixels
	Height         uint16 // Display height in pixels
	Color          byte   // Color type (1=black, 2=red, 3=yellow, 5=7colour, 6=spectra6, 7=red/yellow)
	PCBVariant     byte   // PCB version (e.g., 12 = v1.2)
	DisplayVariant byte   // Display variant ID (maps to display type)
	WriteTime      string // Timestamp when EEPROM was written
}

// GetVariantName returns the human-readable name for the display variant
func (e *EEPROMData) GetVariantName() string {
	name := displayVariantNames[e.DisplayVariant]
	if name == "" {
		return fmt.Sprintf("Unknown (variant %d)", e.DisplayVariant)
	}
	return name
}

// ReadEEPROM reads and parses the EEPROM data from an Inky display
// The EEPROM contains display identification and configuration information
// Returns error if the EEPROM cannot be read or the data is invalid
func ReadEEPROM(i2c common.I2C) (*EEPROMData, error) {
	// Allocate buffer for EEPROM data
	data := make([]byte, eepromSize)

	// Write starting address (0x00) to EEPROM
	err := i2c.Tx(eepromAddress, []byte{0x00}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to set EEPROM read address: %w", err)
	}

	// Small delay to allow EEPROM to respond
	time.Sleep(10 * time.Millisecond)

	// Read EEPROM data
	err = i2c.Tx(eepromAddress, nil, data)
	if err != nil {
		return nil, fmt.Errorf("failed to read EEPROM data: %w", err)
	}

	// Parse and return EEPROM data
	return parseEEPROM(data)
}

// parseEEPROM parses the 29-byte EEPROM structure
func parseEEPROM(data []byte) (*EEPROMData, error) {
	if len(data) < eepromSize {
		return nil, fmt.Errorf("invalid EEPROM data: expected %d bytes, got %d", eepromSize, len(data))
	}

	eeprom := &EEPROMData{
		Width:          binary.LittleEndian.Uint16(data[0:2]),
		Height:         binary.LittleEndian.Uint16(data[2:4]),
		Color:          data[4],
		PCBVariant:     data[5],
		DisplayVariant: data[6],
	}

	// Parse write time (22 bytes, pascal string format)
	// First byte is length, remaining bytes are the string
	timeLen := int(data[7])
	if timeLen > 0 && timeLen <= 21 {
		eeprom.WriteTime = string(data[8 : 8+timeLen])
	} else {
		eeprom.WriteTime = "(not set)"
	}

	return eeprom, nil
}

// getColorType converts EEPROM color code to colorType string
func getColorType(colorCode byte) string {
	switch colorCode {
	case 1:
		return "black"
	case 2:
		return "red"
	case 3:
		return "yellow"
	default:
		return "black" // Default to black for unknown color codes
	}
}
