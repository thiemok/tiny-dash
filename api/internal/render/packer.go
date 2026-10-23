package render

import "image"

// PackImage converts an image to the e-ink packed pixel format.
// Pixels are packed row-major, colorDepth bits per pixel, most significant
// bits first: the leftmost pixel of a byte sits in its high bits, as the
// display controllers expect. The device sends these bytes to the panel
// unchanged, so LSB-first packing would swap neighbouring pixels.
// Each pixel's RGB is mapped to the nearest palette color index.
func PackImage(img image.Image, width, height, colorDepth int, palette Palette) []byte {
	pixelsPerByte := 8 / colorDepth
	totalPixels := width * height
	bufferSize := (totalPixels + pixelsPerByte - 1) / pixelsPerByte
	buffer := make([]byte, bufferSize)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			nearest := palette.NearestColor(uint8(r>>8), uint8(g>>8), uint8(b>>8))

			pixelIndex := y*width + x
			byteIndex := pixelIndex / pixelsPerByte
			partialIndex := pixelIndex % pixelsPerByte

			shift := 8 - (partialIndex+1)*colorDepth
			mask := byte(((1 << colorDepth) - 1) << shift)
			buffer[byteIndex] = (buffer[byteIndex] & ^mask) | ((nearest.Index << shift) & mask)
		}
	}

	return buffer
}
