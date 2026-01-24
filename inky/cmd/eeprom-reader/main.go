package main

import (
	"fmt"
	"time"

	"github.com/thiemok/tiny-dash/inky/pkg/adapters"
	"github.com/thiemok/tiny-dash/inky/pkg/inky"
)

func main() {
	println("Starting EEPROM reader in 2 seconds...")
	time.Sleep(2 * time.Second)
	println()

	println("========================================")
	println("Inky EEPROM Reader")
	println("========================================")
	println()

	println("Configuring hardware...")
	hardware, err := adapters.NewPico2PicoToPiHardware()
	if err != nil {
		println("Error: Failed to configure hardware:", err.Error())
		return
	}
	println("✓ Hardware configured successfully")
	println()

	println("Reading EEPROM at address 0x50...")
	eeprom, err := inky.ReadEEPROM(hardware.I2C)
	if err != nil {
		println("Error: Failed to read EEPROM data:", err.Error())
		println("(Is the Inky display connected and powered?)")
		return
	}

	println("✓ EEPROM read successfully")
	println()

	displayInfo(eeprom)
}

// displayInfo displays human-readable EEPROM information
func displayInfo(eeprom *inky.EEPROMData) {
	println("Detected Display Model:", eeprom.GetVariantName())
	println()
	println("Display Information:")
	fmt.Printf("  Resolution:      %d x %d\n", eeprom.Width, eeprom.Height)
	println("  Color:          ", eeprom.GetColorName())
	println("  PCB Variant:    ", eeprom.GetPCBVariantString())
	fmt.Printf("  Display Variant: %d\n", eeprom.DisplayVariant)
	println("  Write Time:     ", eeprom.WriteTime)
}
