package common

// Framebuffer interface provides pixel-level access to the display buffer
// All display implementations must provide framebuffer operations
type Framebuffer interface {
	// SetPixel sets a pixel at the specified coordinates
	SetPixel(x, y int, color Color)

	// GetPixel returns the color of a pixel at the specified coordinates
	GetPixel(x, y int) Color

	// Width returns the width of the framebuffer in pixels
	Width() int

	// Height returns the height of the framebuffer in pixels
	Height() int

	// ColorDepth returns the amount of bits per pixel
	ColorDepth() int

	// Buffer returns the raw underlying buffer data
	Buffer() []byte
}

// Display represents a generic e-ink display interface
// This interface is implemented by all display types (E640, E673, etc.)
type Display interface {
	// Framebuffer interface for pixel-level access to the display buffer
	Framebuffer

	// Update transfers the framebuffer to the display and triggers a refresh
	Update() error

	// Fill fills the framebuffer with a single color
	Fill(color Color)

	// SupportedColors returns the list of colors supported by this display
	SupportedColors() []Color

	// SupportsColor checks if the display supports a specific color
	SupportsColor(color Color) bool
}

// framebufferImpl provides the concrete implementation of the Framebuffer interface
// Uses packed format internally (2 pixels per byte)
type framebufferImpl struct {
	data       []byte
	width      int
	height     int
	colorDepth int
}

// NewFramebuffer creates a new framebuffer
// width and height indicate the resolution of the buffer
// colorDepth controls how many bits per pixel the buffer uses
func NewFramebuffer(width, height, colorDepth int) Framebuffer {
	pixelPerByte := 8 / colorDepth
	totalPixels := width * height
	// Round up to ensure we have enough bytes even for partial fills
	bufferSize := (totalPixels + pixelPerByte - 1) / pixelPerByte
	return &framebufferImpl{
		width:      width,
		height:     height,
		colorDepth: colorDepth,
		data:       make([]byte, bufferSize),
	}
}

// SetPixel sets a pixel at the specified coordinates
// Handles packed format internally (2 pixels per byte)
func (fb *framebufferImpl) SetPixel(x, y int, color Color) {
	if x < 0 || x >= fb.width || y < 0 || y >= fb.height {
		return // Silently ignore out-of-bounds
	}

	// Calculate position in packed buffer
	pixelIndex := y*fb.width + x
	pixelPerByte := 8 / fb.colorDepth
	byteIndex := pixelIndex / pixelPerByte
	partialIndex := pixelIndex % pixelPerByte

	mask := getBitMask(partialIndex, fb.colorDepth)

	fb.data[byteIndex] = (fb.data[byteIndex] & ^mask) | ((byte(color) << (partialIndex * fb.colorDepth)) & mask)
}

// GetPixel returns the color of a pixel at the specified coordinates
// Handles packed format internally (2 pixels per byte)
func (fb *framebufferImpl) GetPixel(x, y int) Color {
	if x < 0 || x >= fb.width || y < 0 || y >= fb.height {
		return Black // Return default color for out-of-bounds
	}

	// Calculate position in packed buffer
	pixelIndex := y*fb.width + x
	pixelPerByte := 8 / fb.colorDepth
	byteIndex := pixelIndex / pixelPerByte
	partialIndex := pixelIndex % pixelPerByte

	mask := getBitMask(partialIndex, fb.colorDepth)

	return Color((fb.data[byteIndex] & mask) >> (partialIndex * fb.colorDepth))
}

func (fb *framebufferImpl) Width() int {
	return fb.width
}

func (fb *framebufferImpl) Height() int {
	return fb.height
}

// ColorDepth returns the amount of bits per pixel
func (fb *framebufferImpl) ColorDepth() int {
	return fb.colorDepth
}

func (fb *framebufferImpl) Buffer() []byte {
	return fb.data[:]
}

func getBitMask(idx, depth int) byte {
	shift := idx * depth
	base := (1 << depth) - 1
	return byte(base << shift)
}
