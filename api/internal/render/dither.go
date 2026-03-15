package render

import (
	"image"
	"image/color"
	"math"
)

// Dither applies Floyd-Steinberg dithering to src using the given palette.
// Every pixel in the returned image is an exact palette color.
func Dither(src image.Image, palette Palette) *image.RGBA {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Working buffer with float64 RGB per pixel for error diffusion.
	type pixel struct{ r, g, b float64 }
	buf := make([]pixel, w*h)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			i := (y-bounds.Min.Y)*w + (x - bounds.Min.X)
			buf[i] = pixel{float64(r >> 8), float64(g >> 8), float64(b >> 8)}
		}
	}

	out := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			old := buf[i]

			// Clamp to [0, 255]
			cr := clamp(old.r)
			cg := clamp(old.g)
			cb := clamp(old.b)

			nearest := palette.NearestColor(uint8(cr), uint8(cg), uint8(cb))
			out.Set(x, y, color.RGBA{nearest.R, nearest.G, nearest.B, 255})

			// Quantization error
			er := cr - float64(nearest.R)
			eg := cg - float64(nearest.G)
			eb := cb - float64(nearest.B)

			// Distribute error to neighbors
			distribute := func(dx, dy int, factor float64) {
				nx, ny := x+dx, y+dy
				if nx >= 0 && nx < w && ny >= 0 && ny < h {
					j := ny*w + nx
					buf[j].r += er * factor
					buf[j].g += eg * factor
					buf[j].b += eb * factor
				}
			}

			distribute(1, 0, 7.0/16.0)  // right
			distribute(-1, 1, 3.0/16.0) // below-left
			distribute(0, 1, 5.0/16.0)  // below
			distribute(1, 1, 1.0/16.0)  // below-right
		}
	}

	return out
}

func clamp(v float64) float64 {
	return math.Max(0, math.Min(255, math.Round(v)))
}
