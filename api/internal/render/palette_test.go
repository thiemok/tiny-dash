package render

import (
	"image"
	"image/color"
	"testing"
)

func TestCovers(t *testing.T) {
	spectra6 := PaletteFromColors([]byte{0, 1, 2, 3, 5, 6})
	if !spectra6.Covers([]byte{0, 1, 3}) {
		t.Error("Spectra 6 palette should cover black, white, red")
	}
	if spectra6.Covers([]byte{0, 4}) {
		t.Error("Spectra 6 palette has no orange")
	}
	if !spectra6.Covers(nil) {
		t.Error("every palette covers the empty set")
	}
}

func TestQuantizeSnapsWithoutDiffusing(t *testing.T) {
	bw := PaletteFromColors([]byte{0, 1})
	src := image.NewRGBA(image.Rect(0, 0, 4, 1))
	// A mid-dark grey run: dithering would mix in white pixels, snapping must not.
	for x := 0; x < 4; x++ {
		src.Set(x, 0, color.RGBA{100, 100, 100, 255})
	}
	out := Quantize(src, bw)
	for x := 0; x < 4; x++ {
		if got := out.RGBAAt(x, 0); got != (color.RGBA{0, 0, 0, 255}) {
			t.Errorf("pixel %d = %v, want black", x, got)
		}
	}
}
