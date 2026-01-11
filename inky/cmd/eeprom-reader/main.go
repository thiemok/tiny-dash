package main

import (
	"encoding/binary"
	"fmt"
	"machine"
	"time"
)

// EEPROM constants
const (
	eepromAddress = 0x50
	eepromSize    = 29
)

// Display variant names mapping (from Pimoroni's Python implementation)
var displayVariants = map[byte]string{
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

// Color mapping (from Pimoroni's Python implementation)
var colorNames = map[byte]string{
	1: "black",
	2: "red",
	3: "yellow",
	5: "7colour",
	6: "spectra6",
	7: "red/yellow",
}

// EEPROMData represents the parsed EEPROM structure
type EEPROMData struct {
	Width          uint16
	Height         uint16
	Color          byte
	PCBVariant     byte
	DisplayVariant byte
	WriteTime      string
}

func main() {
	// Short delay to allow serial connection to stabilize
	println("Starting EEPROM reader in 2 seconds...")
	time.Sleep(2 * time.Second)
	println()

	println("========================================")
	println("Inky EEPROM Reader")
	println("========================================")
	println()

	// Initialize I2C
	i2c := machine.I2C1
	err := i2c.Configure(machine.I2CConfig{
		Frequency: 100000, // 100 kHz - standard I2C speed
		SDA:       machine.I2C1_SDA_PIN,
		SCL:       machine.I2C1_SCL_PIN,
	})
	if err != nil {
		println("Error: Failed to configure I2C:", err.Error())
		return
	}

	// Read EEPROM data
	println("Reading EEPROM at address 0x50...")
	data := make([]byte, eepromSize)

	// First, write the starting address (0x00) to the EEPROM
	err = i2c.Tx(eepromAddress, []byte{0x00}, nil)
	if err != nil {
		println("Error: Failed to set EEPROM read address:", err.Error())
		println("(Is the Inky display connected and powered?)")
		return
	}

	// Small delay to allow EEPROM to respond
	time.Sleep(10 * time.Millisecond)

	// Read the data
	err = i2c.Tx(eepromAddress, nil, data)
	if err != nil {
		println("Error: Failed to read EEPROM data:", err.Error())
		println("(Is the Inky display connected and powered?)")
		return
	}

	println("✓ EEPROM read successfully")
	println()

	// Parse the EEPROM data
	eeprom := parseEEPROM(data)

	// Display human-readable information
	displayInfo(eeprom)
	println()

	// Display raw hex dump
	displayHexDump(data)
}

// parseEEPROM parses the 29-byte EEPROM structure
func parseEEPROM(data []byte) EEPROMData {
	if len(data) < eepromSize {
		return EEPROMData{}
	}

	eeprom := EEPROMData{
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

	return eeprom
}

// displayInfo displays human-readable EEPROM information
func displayInfo(eeprom EEPROMData) {
	// Get display variant name
	variantName := displayVariants[eeprom.DisplayVariant]
	if variantName == "" {
		variantName = fmt.Sprintf("Unknown (variant %d)", eeprom.DisplayVariant)
	}

	println("Detected Display Model:", variantName)
	println()
	println("Display Information:")
	fmt.Printf("  Resolution:      %d x %d\n", eeprom.Width, eeprom.Height)

	// Get color name
	colorName := colorNames[eeprom.Color]
	if colorName == "" {
		colorName = fmt.Sprintf("unknown (0x%02X)", eeprom.Color)
	}
	println("  Color:          ", colorName)

	// Format PCB variant as decimal version
	fmt.Printf("  PCB Variant:     v%.1f\n", float32(eeprom.PCBVariant)/10.0)
	fmt.Printf("  Display Variant: %d\n", eeprom.DisplayVariant)
	println("  Write Time:     ", eeprom.WriteTime)
}

// displayHexDump displays raw EEPROM data as a hex dump
func displayHexDump(data []byte) {
	println("========================================")
	println("Raw EEPROM Data (29 bytes):")
	println("========================================")

	for i := 0; i < len(data); i += 16 {
		// Print address
		fmt.Printf("0x%02X: ", i)

		// Print hex values
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		for j := i; j < end; j++ {
			fmt.Printf("%02X ", data[j])
		}

		// Print ASCII representation (printable chars only)
		print(" |")
		for j := i; j < end; j++ {
			if data[j] >= 32 && data[j] <= 126 {
				fmt.Printf("%c", data[j])
			} else {
				print(".")
			}
		}
		println("|")
	}
	println()
}
