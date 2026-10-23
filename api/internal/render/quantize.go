package render

import (
	"image"
	"image/color"
)

// Quantize maps every pixel of src to its nearest palette color without
// diffusing the error. For flat UI drawn in palette colors this keeps edges
// crisp, where Dither would scatter antialiasing pixels into speckle.
func Quantize(src image.Image, palette Palette) *image.RGBA {
	bounds := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			nearest := palette.NearestColor(uint8(r>>8), uint8(g>>8), uint8(b>>8))
			out.Set(x-bounds.Min.X, y-bounds.Min.Y, color.RGBA{nearest.R, nearest.G, nearest.B, 255})
		}
	}

	return out
}
