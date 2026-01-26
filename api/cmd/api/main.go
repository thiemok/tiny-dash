package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func main() {
	http.HandleFunc("/api/dashboard/image", handleImageRequest)
	
	port := ":8080"
	log.Printf("Starting API server on %s", port)
	log.Printf("Endpoint: GET /api/dashboard/image?width=X&height=Y&colorDepth=Z&colors=R,G,B,...")
	
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleImageRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %s", r.URL.String())
	
	// Parse query parameters
	width, err := strconv.Atoi(r.URL.Query().Get("width"))
	if err != nil || width <= 0 {
		http.Error(w, "Invalid width parameter", http.StatusBadRequest)
		return
	}
	
	height, err := strconv.Atoi(r.URL.Query().Get("height"))
	if err != nil || height <= 0 {
		http.Error(w, "Invalid height parameter", http.StatusBadRequest)
		return
	}
	
	colorDepth, err := strconv.Atoi(r.URL.Query().Get("colorDepth"))
	if err != nil || colorDepth <= 0 {
		http.Error(w, "Invalid colorDepth parameter", http.StatusBadRequest)
		return
	}
	
	colorsParam := r.URL.Query().Get("colors")
	if colorsParam == "" {
		http.Error(w, "Missing colors parameter", http.StatusBadRequest)
		return
	}
	
	colorStrs := strings.Split(colorsParam, ",")
	colors := make([]byte, len(colorStrs))
	for i, cs := range colorStrs {
		c, err := strconv.Atoi(strings.TrimSpace(cs))
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid color value: %s", cs), http.StatusBadRequest)
			return
		}
		colors[i] = byte(c)
	}
	
	log.Printf("Generating image: %dx%d, depth=%d, colors=%v", width, height, colorDepth, colors)
	
	// Generate image
	imageData := generateImage(width, height, colorDepth, colors)
	
	// Return packed binary data
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(imageData)))
	w.WriteHeader(http.StatusOK)
	w.Write(imageData)
	
	log.Printf("Sent %d bytes", len(imageData))
}

func generateImage(width, height, colorDepth int, colors []byte) []byte {
	// Create framebuffer matching e-ink packing format
	pixelsPerByte := 8 / colorDepth
	totalPixels := width * height
	bufferSize := (totalPixels + pixelsPerByte - 1) / pixelsPerByte
	buffer := make([]byte, bufferSize)
	
	// Create color palette for the image
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	
	// Draw color bars (vertical bars, one per color)
	barWidth := width / len(colors)
	for i, c := range colors {
		startX := i * barWidth
		endX := (i + 1) * barWidth
		if i == len(colors)-1 {
			endX = width // Last bar fills remaining width
		}
		
		// Fill bar with color
		for y := 0; y < height; y++ {
			for x := startX; x < endX; x++ {
				// Set color in image (for text rendering)
				img.Set(x, y, getDisplayColor(c))
			}
		}
	}
	
	// Draw timestamp text
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	drawText(img, 10, 30, timestamp, color.Black)
	
	// Convert image to packed format
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Get color from image
			r, g, b, _ := img.At(x, y).RGBA()
			colorValue := mapRGBToColorValue(r, g, b, colors)
			
			// Pack pixel into buffer (same logic as inky framebuffer)
			pixelIndex := y*width + x
			byteIndex := pixelIndex / pixelsPerByte
			partialIndex := pixelIndex % pixelsPerByte
			
			mask := getBitMask(partialIndex, colorDepth)
			buffer[byteIndex] = (buffer[byteIndex] & ^mask) | ((colorValue << (partialIndex * colorDepth)) & mask)
		}
	}
	
	return buffer
}

func getBitMask(idx, depth int) byte {
	shift := idx * depth
	base := (1 << depth) - 1
	return byte(base << shift)
}

func getDisplayColor(colorValue byte) color.RGBA {
	// Map color values to RGB for rendering
	// These are approximate - actual e-ink colors vary
	switch colorValue {
	case 0:
		return color.RGBA{0, 0, 0, 255}       // Black
	case 1:
		return color.RGBA{255, 255, 255, 255} // White
	case 2:
		return color.RGBA{0, 255, 0, 255}     // Green
	case 3:
		return color.RGBA{0, 0, 255, 255}     // Blue
	case 4:
		return color.RGBA{255, 0, 0, 255}     // Red
	case 5:
		return color.RGBA{255, 255, 0, 255}   // Yellow
	case 6:
		return color.RGBA{255, 165, 0, 255}   // Orange
	default:
		return color.RGBA{128, 128, 128, 255} // Gray
	}
}

func mapRGBToColorValue(r, g, b uint32, colors []byte) byte {
	// Simple color mapping - find closest color in palette
	// For color bars, this should match exactly
	r8 := byte(r >> 8)
	g8 := byte(g >> 8)
	b8 := byte(b >> 8)
	
	// Black
	if r8 < 50 && g8 < 50 && b8 < 50 {
		return 0
	}
	// White
	if r8 > 200 && g8 > 200 && b8 > 200 {
		return 1
	}
	// Green
	if g8 > 200 && r8 < 100 && b8 < 100 {
		return 2
	}
	// Blue
	if b8 > 200 && r8 < 100 && g8 < 100 {
		return 3
	}
	// Red
	if r8 > 200 && g8 < 100 && b8 < 100 {
		return 4
	}
	// Yellow
	if r8 > 200 && g8 > 200 && b8 < 100 {
		return 5
	}
	// Orange
	if r8 > 200 && g8 > 100 && g8 < 200 && b8 < 100 {
		return 6
	}
	
	// Default to first color in palette
	if len(colors) > 0 {
		return colors[0]
	}
	return 0
}

func drawText(img *image.RGBA, x, y int, text string, col color.Color) {
	point := fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(text)
}
