package inky

import (
	"fmt"

	"github.com/thiemok/tiny-dash/inky/pkg/inky/common"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/ac073tc1a"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/e640"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/e673"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/el133uf1"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/jd79661"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/jd79668"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/phat"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/ssd1608"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/ssd1683"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/uc8159"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/what"
)

// Auto detects and initializes the connected Inky display via EEPROM
// Reads the EEPROM to identify the display type and returns an initialized Display
// Returns error if EEPROM cannot be read or display type is not supported
func Auto(config common.InkyConfig) (common.Display, error) {
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
		colorType := getColorType(eeprom.Color)
		return phat.New(config, colorType)

	case 2, 3, 6, 7, 8:
		// InkyWHAT (400x300) - variants 2, 3, 6, 7, 8
		colorType := getColorType(eeprom.Color)
		return what.New(config, colorType)

	case 10, 11, 12:
		// InkyPHAT_SSD1608 (250x122) - variants 10, 11, 12
		colorType := getColorType(eeprom.Color)
		return ssd1608.New(config, colorType)

	case 14:
		// InkyUC8159 (600x448) - variant 14
		return uc8159.New(config, 600, 448)

	case 15, 16:
		// InkyUC8159 (640x400) - variants 15, 16
		return uc8159.New(config, 640, 400)

	case 17, 18, 19:
		// InkyWHAT_SSD1683 (400x300) - variants 17, 18, 19
		colorType := getColorType(eeprom.Color)
		return ssd1683.New(config, colorType)

	case 20:
		// InkyAC073TC1A (800x480) - variant 20
		return ac073tc1a.New(config)

	case 21:
		// InkyEL133UF1 (1600x1200) - variant 21
		// NOTE: Requires config.CS1 to be set for dual chip select
		return el133uf1.New(config)

	case 22:
		// InkyE673 - Spectra 6 7.3\" 800x480 - variant 22
		return e673.New(config)

	case 23:
		// InkyJD79661 (250x122) - variant 23
		return jd79661.New(config, "red/yellow")

	case 24:
		// InkyJD79668 (400x300) - variant 24
		return jd79668.New(config, "red/yellow")

	case 25:
		// InkyE640 - Spectra 6 4.0\" 400x600 - variant 25
		return e640.New(config)

	default:
		return nil, fmt.Errorf("unknown display variant %d (resolution: %dx%d)", eeprom.DisplayVariant, eeprom.Width, eeprom.Height)
	}
}
