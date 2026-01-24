package e673

import "github.com/thiemok/tiny-dash/inky/pkg/inky/common"

// HasButtons returns true if this display has button support
func (d *E673Display) HasButtons() bool {
	return d.buttons != nil
}

// HasLED returns true if this display has LED support
func (d *E673Display) HasLED() bool {
	return d.led != nil
}

// GetButtons returns the button controller (nil if not supported)
func (d *E673Display) GetButtons() *common.ButtonController {
	return d.buttons
}

// GetLED returns the LED controller (nil if not supported)
func (d *E673Display) GetLED() *common.LEDController {
	return d.led
}
