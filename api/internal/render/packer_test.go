package render

import (
	"image"
	"image/color"
	"testing"
)

func TestPackImageLeftPixelInHighNibble(t *testing.T) {
	palette := PaletteFromColors([]byte{0, 1, 2, 3, 5, 6})
	img := image.NewRGBA(image.Rect(0, 0, 4, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})   // red, index 3
	img.Set(1, 0, color.RGBA{0, 0, 255, 255})   // blue, index 5
	img.Set(2, 0, color.RGBA{0, 0, 0, 255})     // black, index 0
	img.Set(3, 0, color.RGBA{255, 255, 0, 255}) // yellow, index 2

	got := PackImage(img, 4, 1, 4, palette)
	// Pimoroni's E673 driver: buf[::2] << 4 | buf[1::2]
	want := []byte{0x35, 0x02}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("PackImage = % X, want % X", got, want)
	}
}
