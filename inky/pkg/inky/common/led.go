package common

import "fmt"

// LEDController manages a single LED output
// Provides simple on/off/toggle operations
type LEDController struct {
	pin   Pin
	state bool
}

// NewLEDController creates a new LED controller for the given pin
// Returns nil if the pin is nil
func NewLEDController(pin Pin) (*LEDController, error) {
	if pin == nil {
		return nil, nil // No LED available
	}

	// Configure pin as output
	if err := pin.Configure(PinOutput); err != nil {
		return nil, fmt.Errorf("failed to configure LED pin: %w", err)
	}

	// Initial state: off
	pin.Set(false)

	return &LEDController{
		pin:   pin,
		state: false,
	}, nil
}

// On turns the LED on
func (lc *LEDController) On() error {
	if lc == nil || lc.pin == nil {
		return fmt.Errorf("LED controller not initialized")
	}
	lc.pin.Set(true)
	lc.state = true
	return nil
}

// Off turns the LED off
func (lc *LEDController) Off() error {
	if lc == nil || lc.pin == nil {
		return fmt.Errorf("LED controller not initialized")
	}
	lc.pin.Set(false)
	lc.state = false
	return nil
}

// Toggle toggles the LED state
func (lc *LEDController) Toggle() error {
	if lc == nil || lc.pin == nil {
		return fmt.Errorf("LED controller not initialized")
	}
	lc.state = !lc.state
	lc.pin.Set(lc.state)
	return nil
}

// IsOn returns true if the LED is currently on
func (lc *LEDController) IsOn() bool {
	if lc == nil {
		return false
	}
	return lc.state
}
