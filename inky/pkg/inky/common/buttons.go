package common

import (
	"fmt"
	"sync/atomic"
	"time"
)

// nowNano is the clock the interrupt debounce runs on; tests replace it.
var nowNano = func() int64 { return time.Now().UnixNano() }

// Button represents a single button with debouncing state
type Button struct {
	pin          Pin
	lastState    bool
	currentState bool
	lastDebounce time.Time
	pressed      bool // Edge detection: true if button was pressed since last check

	// With interrupts enabled, onEdge latches presses into latched instead
	// of Poll setting pressed.
	irq      bool
	debounce int64 // ns
	lastEdge int64 // ns, written only by onEdge
	latched  atomic.Bool
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

		// The buttons only pull the pin to ground; without the pull-up a
		// released button floats and reads as random presses.
		if err := pin.Configure(PinInputPullUp); err != nil {
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

// EnableInterrupts latches presses from the pins' change interrupts, so a
// press counts even while nothing calls Poll, e.g. during a display refresh;
// WasPressed reports it afterwards. Buttons whose pin is no InterruptPin keep
// relying on Poll. It returns how many buttons are interrupt-driven.
func (bc *ButtonController) EnableInterrupts() (int, error) {
	if bc == nil {
		return 0, fmt.Errorf("button controller not initialized")
	}
	n := 0
	for i, button := range bc.buttons {
		if button == nil {
			continue
		}
		ip, ok := button.pin.(InterruptPin)
		if !ok {
			continue
		}
		button.debounce = int64(bc.debounceTime)
		button.lastEdge = nowNano()
		if err := ip.SetChangeInterrupt(button.onEdge); err != nil {
			return n, fmt.Errorf("enabling interrupt of button %d: %w", i, err)
		}
		button.irq = true
		n++
	}
	return n, nil
}

// onEdge runs in interrupt context on every level change. A falling edge
// after debounce without any edge is a press; the bounces of a press or a
// release follow each other more closely and are ignored.
func (b *Button) onEdge() {
	now := nowNano()
	if !b.pin.Get() && now-b.lastEdge > b.debounce {
		b.latched.Store(true)
	}
	b.lastEdge = now
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
			if button.currentState && !reading && !button.irq {
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

	if button.irq {
		return button.latched.Swap(false)
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
