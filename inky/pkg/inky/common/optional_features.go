package common

// OptionalFeatures interface for displays with additional hardware (buttons, LED)
// Not all displays support these features. Use type assertion to check if a display
// implements this interface and then check the Has* methods before accessing features.
type OptionalFeatures interface {
	Display // Embed Display interface

	// HasButtons returns true if the display has button support
	HasButtons() bool

	// HasLED returns true if the display has LED support
	HasLED() bool

	// GetButtons returns the button controller (nil if not supported)
	GetButtons() *ButtonController

	// GetLED returns the LED controller (nil if not supported)
	GetLED() *LEDController
}
