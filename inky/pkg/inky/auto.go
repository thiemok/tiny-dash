package inky

import "fmt"

// Auto detects and initializes the connected Inky display via EEPROM
// Reads the EEPROM to identify the display type and returns an initialized Display
// Returns error if EEPROM cannot be read or display type is not supported
func Auto(config InkyConfig) (Display, error) {
	// Read EEPROM to identify display
	eeprom, err := ReadEEPROM(config.I2C)
	if err != nil {
		return nil, fmt.Errorf("failed to read display EEPROM: %w", err)
	}

	// Map display variant to appropriate constructor
	// Based on Pimoroni's Python implementation
	switch eeprom.DisplayVariant {
	case 1, 4, 5:
		// InkyPHAT (212x104) - variants 1, 4, 5
		return nil, fmt.Errorf("InkyPHAT (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 2, 3, 6, 7, 8:
		// InkyWHAT (400x300) - variants 2, 3, 6, 7, 8
		return nil, fmt.Errorf("InkyWHAT (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 10, 11, 12:
		// InkyPHAT_SSD1608 (212x104) - variants 10, 11, 12
		return nil, fmt.Errorf("InkyPHAT_SSD1608 (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 14:
		// InkyUC8159 (600x448) - variant 14
		return nil, fmt.Errorf("InkyUC8159 600x448 (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 15, 16:
		// InkyUC8159 (640x400) - variants 15, 16
		return nil, fmt.Errorf("InkyUC8159 640x400 (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 17, 18, 19:
		// InkyWHAT_SSD1683 (400x300) - variants 17, 18, 19
		return nil, fmt.Errorf("InkyWHAT_SSD1683 (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 20:
		// InkyAC073TC1A (800x480) - variant 20
		return nil, fmt.Errorf("InkyAC073TC1A (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 21:
		// InkyEL133UF1 (1600x1200) - variant 21
		return nil, fmt.Errorf("InkyEL133UF1 (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 22:
		// InkyE673 - Spectra 6 7.3" 800x480 - variant 22
		return NewE673(config)

	case 23:
		// InkyJD79661 (250x122) - variant 23
		return nil, fmt.Errorf("InkyJD79661 (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 24:
		// InkyJD79668 (400x300) - variant 24
		return nil, fmt.Errorf("InkyJD79668 (variant %d: %s) not yet implemented", eeprom.DisplayVariant, eeprom.GetVariantName())

	case 25:
		// InkyE640 - Spectra 6 4.0" 400x600 - variant 25
		return NewE640(config)

	default:
		return nil, fmt.Errorf("unknown display variant %d (resolution: %dx%d)", eeprom.DisplayVariant, eeprom.Width, eeprom.Height)
	}
}
