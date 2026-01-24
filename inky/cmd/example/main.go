package main

import (
	"time"

	"github.com/thiemok/tiny-dash/inky/pkg/adapters"
	inkyCommon "github.com/thiemok/tiny-dash/inky/pkg/inky/common"
	"github.com/thiemok/tiny-dash/inky/pkg/inky/drivers/e673"
)

func main() {
	// Short delay to allow serial connection to stabilize
	println("Starting in 2 seconds...")
	time.Sleep(time.Second * 2)
	println()

	// Print immediately to confirm the program is running
	println("========================================")
	println("Inky Impression Example")
	println("========================================")
	println()

	// Configure hardware for Pico 2 W + Pico-to-Pi adapter
	println("Configuring hardware...")
	hardware, err := adapters.NewPico2PicoToPiHardware()
	if err != nil {
		println("Error: Failed to configure hardware:", err.Error())
		return
	}
	println("✓ Hardware configured successfully")
	println()

	// Auto-detect and initialize display via EEPROM
	println("Detecting display via EEPROM...")
	var display inkyCommon.Display
	// display, err := inky.Auto(*hardware)
	display, err = e673.New(*hardware)
	if err != nil {
		println("Error:", err.Error())
		return
	}
	println("✓ Display detected and initialized successfully")
	println()

	// Check for optional features (buttons and LED)
	var buttons *inkyCommon.ButtonController
	var led *inkyCommon.LEDController

	if optDisplay, ok := display.(inkyCommon.OptionalFeatures); ok {
		if optDisplay.HasButtons() {
			buttons = optDisplay.GetButtons()
			println("✓ Buttons detected:", buttons.ButtonCount(), "buttons")
		}
		if optDisplay.HasLED() {
			led = optDisplay.GetLED()
			println("✓ LED detected")
		}
	}
	println()

	// Generate initial test pattern
	println("Generating test pattern...")
	colorOffset := 0
	generateTestPattern(display, colorOffset)
	println("✓ Test pattern generated")
	println()

	// Update display with LED blinking if available
	updateDisplayWithLED(display, led)

	println("========================================")
	println("Test pattern successfully displayed!")
	println("You should see vertical color bars for each color supported by your display")
	if buttons != nil {
		println()
		println("Button controls:")
		println("  Button A: Rotate colors forward")
		println("  Button B: Rotate colors backward")
		println("  Button C: Reset to original pattern")
		println("  Button D: Toggle LED (if available)")
	}
	println("========================================")
	println()

	// If no buttons, just run with periodic heartbeat
	if buttons == nil {
		println("No buttons detected - running with heartbeat every 5 seconds...")
		counter := 0
		for {
			println("Heartbeat:", counter)
			counter++
			time.Sleep(time.Second * 5)
		}
	}

	// Main loop with button polling
	println("Button polling active - press buttons to interact...")
	println()
	ledState := false
	for {
		// Poll buttons (synchronous, as requested)
		if err := buttons.Poll(); err != nil {
			println("Error polling buttons:", err.Error())
		}

		// Check each button
		if buttons.WasPressed(0) {
			// Button A: Rotate colors forward
			println("Button A pressed - rotating colors forward")
			colorOffset = (colorOffset + 1) % len(display.SupportedColors())
			generateTestPattern(display, colorOffset)
			updateDisplayWithLED(display, led)
			println("✓ Display updated")
			println()
		}

		if buttons.WasPressed(1) {
			// Button B: Rotate colors backward
			println("Button B pressed - rotating colors backward")
			colorOffset = (colorOffset - 1 + len(display.SupportedColors())) % len(display.SupportedColors())
			generateTestPattern(display, colorOffset)
			updateDisplayWithLED(display, led)
			println("✓ Display updated")
			println()
		}

		if buttons.WasPressed(2) {
			// Button C: Reset to original pattern
			println("Button C pressed - resetting to original pattern")
			colorOffset = 0
			generateTestPattern(display, colorOffset)
			updateDisplayWithLED(display, led)
			println("✓ Display updated")
			println()
		}

		if buttons.WasPressed(3) {
			// Button D: Toggle LED
			if led != nil {
				ledState = !ledState
				if ledState {
					println("Button D pressed - LED ON")
					led.On()
				} else {
					println("Button D pressed - LED OFF")
					led.Off()
				}
			} else {
				println("Button D pressed - but no LED available")
			}
			println()
		}

		// Sleep for a short time to avoid busy-waiting
		time.Sleep(50 * time.Millisecond)
	}
}

// updateDisplayWithLED updates the display and blinks the LED during the update
func updateDisplayWithLED(display inkyCommon.Display, led *inkyCommon.LEDController) {
	println("Updating display...")
	println("(This may take 30-40 seconds depending on your display)")

	// Start LED blinking in background if available
	var stopBlink chan bool
	if led != nil {
		stopBlink = make(chan bool)
		go func() {
			for {
				select {
				case <-stopBlink:
					led.Off()
					return
				default:
					led.Toggle()
					time.Sleep(500 * time.Millisecond)
				}
			}
		}()
	}

	// Update display
	if err := display.Update(); err != nil {
		println("Error:", err.Error())
		if stopBlink != nil {
			stopBlink <- true
		}
		return
	}

	// Stop LED blinking
	if stopBlink != nil {
		stopBlink <- true
	}

	println("✓ Display updated successfully!")
}

// generateTestPattern creates a test pattern with vertical color bars
// colorOffset allows rotating the colors to demonstrate button interaction
func generateTestPattern(display inkyCommon.Display, colorOffset int) {
	colors := display.SupportedColors()

	// Get display dimensions
	width := display.Width()
	height := display.Height()

	// Calculate bar width
	barWidth := width / len(colors)

	// Fill framebuffer with vertical color bars
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Determine which color bar this pixel belongs to
			colorIndex := x / barWidth

			if colorIndex >= len(colors) {
				colorIndex = len(colors) - 1
			}

			// Apply color offset (for button-driven rotation)
			rotatedIndex := (colorIndex + colorOffset) % len(colors)

			// Set pixel color directly in framebuffer
			display.SetPixel(x, y, colors[rotatedIndex])
		}
	}
}
