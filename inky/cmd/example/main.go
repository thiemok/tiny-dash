package main

import (
	"time"

	"github.com/thiemok/tiny-dash/inky/pkg/inky"
)

func main() {
	// Short delay to allow serial connection to stabilize
	println("Starting in 2 seconds...")
	time.Sleep(time.Second * 2)
	println()

	// Print immediately to confirm the program is running
	println("========================================")
	println("Inky Impression Spectra 6 Example")
	println("========================================")
	println()

	// Create and initialize display - auto-configures everything!
	// Use NewE673() for 7.3" 800x480 display
	// Use NewE640() for 4.0" 400x600 display
	println("Initializing display...")
	display, err := inky.NewE673() // Change to NewE640() if using the E640 display
	if err != nil {
		println("Error:", err.Error())
		return
	}
	println("✓ Display initialized successfully")
	println()

	// Get framebuffer for pixel access
	fb := display.GetFramebuffer()
	
	// Generate test pattern (6 vertical color bars)
	println("Generating test pattern (6 color bars)...")
	generateTestPattern(fb, display)
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
	println("You should see 6 vertical color bars:")
	println("  1. Black")
	println("  2. White")
	println("  3. Red")
	println("  4. Yellow")
	println("  5. Blue")
	println("  6. Green")
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
func generateTestPattern(fb inky.Framebuffer, display inky.Display) {
	// Define the 6 colors in order
	colors := []inky.Color{
		inky.Black,
		inky.White,
		inky.Red,
		inky.Yellow,
		inky.Blue,
		inky.Green,
	}

	// Get display dimensions
	width := display.Width()
	height := display.Height()

	// Calculate bar width
	barWidth := width / 6

	// Fill framebuffer with vertical color bars
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Determine which color bar this pixel belongs to
			colorIndex := x / barWidth
			if colorIndex >= 6 {
				colorIndex = 5 // Handle any rounding at the edge
			}

			// Set pixel color directly in framebuffer
			fb.SetPixel(x, y, colors[colorIndex])
		}
	}
}
