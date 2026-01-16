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
	//display, err := inky.Auto(*hardware)
	display, err := e673.New(*hardware)
	if err != nil {
		println("Error:", err.Error())
		return
	}
	println("✓ Display detected and initialized successfully")
	println()

	// Generate test pattern
	println("Generating test pattern...")
	generateTestPattern(display)
	println("✓ Test pattern generated")
	println()

	// Update display (transfer + refresh combined)
	println("Updating display...")
	println("(This will take approximately 30-40 seconds)")
	if err := display.Update(); err != nil {
		println("Error:", err.Error())
		return
	}
	println("✓ Display updated successfully!")
	println()

	println("========================================")
	println("Test pattern successfully displayed!")
	println("You should see vertical color bars for each color supported by your display")
	println("========================================")
	println()

	// Keep running with periodic heartbeat to confirm program is alive
	println("Program running - heartbeat every 5 seconds...")
	counter := 0
	for {
		println("Heartbeat:", counter)
		counter++
		time.Sleep(time.Second * 5)
	}
}

// generateTestPattern creates a test pattern with 6 vertical color bars
// Each bar width is calculated based on the display width
// Uses the Framebuffer API - no allocations, works directly with display buffer
func generateTestPattern(display inkyCommon.Display) {
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

			// Set pixel color directly in framebuffer
			display.SetPixel(x, y, colors[colorIndex])
		}
	}
}
