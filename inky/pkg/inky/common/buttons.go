package common

import (
	"fmt"
	"time"
)

// Button represents a single button with debouncing state
type Button struct {
	pin          Pin
	lastState    bool
	currentState bool
	lastDebounce time.Time
	pressed      bool // Edge detection: true if button was pressed since last check
}

// ButtonController manages multiple buttons with debouncing
// Uses polling approach - call Poll() from your main loop
type ButtonController struct {
	buttons      []*Button
	debounceTime time.Duration
}

// NewButtonController creates a new button controller for the given pins
// Returns nil if no button pins are provided
// debounceTime is the minimum time between state changes (typically 50ms)
func NewButtonController(pins []Pin, debounceTime time.Duration) (*ButtonController, error) {
	if len(pins) == 0 {
		return nil, nil // No buttons available
	}

	buttons := make([]*Button, len(pins))
	for i, pin := range pins {
		if pin == nil {
			continue
		}

		// Configure pin as input
		if err := pin.Configure(PinInput); err != nil {
			return nil, fmt.Errorf("failed to configure button pin %d: %w", i, err)
		}

		buttons[i] = &Button{
			pin:          pin,
			lastState:    pin.Get(),
			currentState: pin.Get(),
			lastDebounce: time.Now(),
			pressed:      false,
		}
	}

	return &ButtonController{
		buttons:      buttons,
		debounceTime: debounceTime,
	}, nil
}

// Poll reads all button states and updates debounced state
// Call this method regularly from your main loop (e.g., every 10-50ms)
// This is synchronous and returns immediately after reading all pins
func (bc *ButtonController) Poll() error {
	if bc == nil {
		return fmt.Errorf("button controller not initialized")
	}

	now := time.Now()

	for _, button := range bc.buttons {
		if button == nil || button.pin == nil {
			continue
		}

		// Read current pin state
		reading := button.pin.Get()

		// If state changed, reset debounce timer
		if reading != button.lastState {
			button.lastDebounce = now
			button.lastState = reading
		}

		// If enough time has passed, update current state
		if now.Sub(button.lastDebounce) > bc.debounceTime {
			// Detect rising edge (button press)
			// Assuming active-low buttons (pressed = false/LOW)
			if button.currentState && !reading {
				button.pressed = true
			}
			button.currentState = reading
		}
	}

	return nil
}

// IsPressed returns true if the button is currently pressed (after debouncing)
// Assuming active-low buttons (pressed = LOW/false)
func (bc *ButtonController) IsPressed(buttonIndex int) bool {
	if bc == nil || buttonIndex < 0 || buttonIndex >= len(bc.buttons) {
		return false
	}

	button := bc.buttons[buttonIndex]
	if button == nil {
		return false
	}

	// Active-low: button is pressed when pin is LOW (false)
	return !button.currentState
}

// WasPressed returns true if the button was pressed since the last check
// This provides edge detection - returns true only once per button press
// Call this after Poll() to check for button events
func (bc *ButtonController) WasPressed(buttonIndex int) bool {
	if bc == nil || buttonIndex < 0 || buttonIndex >= len(bc.buttons) {
		return false
	}

	button := bc.buttons[buttonIndex]
	if button == nil {
		return false
	}

	// Check and clear the pressed flag
	if button.pressed {
		button.pressed = false
		return true
	}

	return false
}

// ButtonCount returns the number of buttons managed by this controller
func (bc *ButtonController) ButtonCount() int {
	if bc == nil {
		return 0
	}
	return len(bc.buttons)
}
