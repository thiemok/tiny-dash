package inky

// Display represents a generic e-ink display interface
// This interface is implemented by all display types (E640, E673, etc.)
type Display interface {
	// GetFramebuffer returns a Framebuffer for pixel-level access to the display buffer
	GetFramebuffer() Framebuffer

	// Update transfers the framebuffer to the display and triggers a refresh
	Update() error

	// Clear fills the framebuffer with a single color
	Clear(color Color)

	// Width returns the display width in pixels
	Width() int

	// Height returns the display height in pixels
	Height() int

	// SupportedColors returns the list of colors supported by this display
	SupportedColors() []Color

	// SupportsColor checks if the display supports a specific color
	SupportsColor(color Color) bool
}

// Framebuffer provides pixel-level access to the display buffer
// Uses packed format internally (2 pixels per byte)
type Framebuffer struct {
	data   []byte
	width  int
	height int
}

// SetPixel sets a pixel at the specified coordinates
// Handles packed format internally (2 pixels per byte)
func (fb *Framebuffer) SetPixel(x, y int, color Color) {
	if x < 0 || x >= fb.width || y < 0 || y >= fb.height {
		return // Silently ignore out-of-bounds
	}

	// Calculate position in packed buffer
	pixelIndex := y*fb.width + x
	byteIndex := pixelIndex / 2
	isHighNibble := (pixelIndex % 2) == 0

	if isHighNibble {
		// Store in high nibble (bits 4-7)
		fb.data[byteIndex] = (fb.data[byteIndex] & 0x0F) | (byte(color) << 4)
	} else {
		// Store in low nibble (bits 0-3)
		fb.data[byteIndex] = (fb.data[byteIndex] & 0xF0) | byte(color)
	}
}

// GetPixel returns the color of a pixel at the specified coordinates
// Handles packed format internally (2 pixels per byte)
func (fb *Framebuffer) GetPixel(x, y int) Color {
	if x < 0 || x >= fb.width || y < 0 || y >= fb.height {
		return Black // Return default color for out-of-bounds
	}

	// Calculate position in packed buffer
	pixelIndex := y*fb.width + x
	byteIndex := pixelIndex / 2
	isHighNibble := (pixelIndex % 2) == 0

	if isHighNibble {
		// Extract from high nibble (bits 4-7)
		return Color((fb.data[byteIndex] >> 4) & 0x0F)
	}
	// Extract from low nibble (bits 0-3)
	return Color(fb.data[byteIndex] & 0x0F)
}
