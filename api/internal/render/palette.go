package render

import "math"

// PaletteEntry maps a display color index to its RGB representation.
type PaletteEntry struct {
	Index byte
	R, G, B uint8
}

// Palette is a set of colors available on the display.
type Palette []PaletteEntry

// Canonical color index to RGB mapping, matching inky/pkg/inky/common/colors.go.
var canonicalColors = map[byte][3]uint8{
	0: {0, 0, 0},       // Black
	1: {255, 255, 255},  // White
	2: {255, 255, 0},    // Yellow
	3: {255, 0, 0},      // Red
	4: {255, 140, 0},    // Orange
	5: {0, 0, 255},      // Blue
	6: {0, 255, 0},      // Green
	7: {255, 255, 255},  // Clean
}

// PaletteFromColors builds a palette from the display's reported color indices.
func PaletteFromColors(colorIndices []byte) Palette {
	p := make(Palette, len(colorIndices))
	for i, idx := range colorIndices {
		rgb, ok := canonicalColors[idx]
		if !ok {
			rgb = [3]uint8{128, 128, 128}
		}
		p[i] = PaletteEntry{Index: idx, R: rgb[0], G: rgb[1], B: rgb[2]}
	}
	return p
}

// NearestColor returns the palette entry closest to the given RGB value
// using Euclidean distance in RGB space.
func (p Palette) NearestColor(r, g, b uint8) PaletteEntry {
	best := p[0]
	bestDist := math.MaxFloat64
	for _, e := range p {
		dr := float64(r) - float64(e.R)
		dg := float64(g) - float64(e.G)
		db := float64(b) - float64(e.B)
		dist := dr*dr + dg*dg + db*db
		if dist < bestDist {
			bestDist = dist
			best = e
		}
	}
	return best
}
