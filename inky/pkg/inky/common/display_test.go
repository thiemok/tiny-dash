package common

import (
	"testing"
)

// TestNewFramebuffer tests framebuffer creation with various configurations
func TestNewFramebuffer(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		colorDepth int
		wantSize   int
	}{
		{
			name:       "1-bit 8x8",
			width:      8,
			height:     8,
			colorDepth: 1,
			wantSize:   8, // 64 pixels / 8 pixels per byte
		},
		{
			name:       "2-bit 8x8",
			width:      8,
			height:     8,
			colorDepth: 2,
			wantSize:   16, // 64 pixels / 4 pixels per byte
		},
		{
			name:       "4-bit 8x8",
			width:      8,
			height:     8,
			colorDepth: 4,
			wantSize:   32, // 64 pixels / 2 pixels per byte
		},
		{
			name:       "8-bit 8x8",
			width:      8,
			height:     8,
			colorDepth: 8,
			wantSize:   64, // 64 pixels / 1 pixel per byte
		},
		{
			name:       "4-bit 640x400",
			width:      640,
			height:     400,
			colorDepth: 4,
			wantSize:   128000, // 256000 pixels / 2 pixels per byte
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := NewFramebuffer(tt.width, tt.height, tt.colorDepth)
			
			if fb.Width() != tt.width {
				t.Errorf("Width() = %d, want %d", fb.Width(), tt.width)
			}
			
			if fb.Height() != tt.height {
				t.Errorf("Height() = %d, want %d", fb.Height(), tt.height)
			}
			
			if fb.ColorDepth() != tt.colorDepth {
				t.Errorf("ColorDepth() = %d, want %d", fb.ColorDepth(), tt.colorDepth)
			}
			
			if len(fb.Buffer()) != tt.wantSize {
				t.Errorf("Buffer size = %d, want %d", len(fb.Buffer()), tt.wantSize)
			}
		})
	}
}

// TestSetGetPixelRoundTrip tests that SetPixel followed by GetPixel returns the same color
func TestSetGetPixelRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		colorDepth int
		testCases  []struct {
			x     int
			y     int
			color Color
		}
	}{
		{
			name:       "4-bit single pixel",
			width:      4,
			height:     4,
			colorDepth: 4,
			testCases: []struct {
				x     int
				y     int
				color Color
			}{
				{0, 0, Black},
				{1, 0, White},
				{2, 0, Yellow},
				{3, 0, Red},
			},
		},
		{
			name:       "4-bit multiple positions",
			width:      8,
			height:     8,
			colorDepth: 4,
			testCases: []struct {
				x     int
				y     int
				color Color
			}{
				{0, 0, Black},
				{1, 0, White},
				{0, 1, Yellow},
				{1, 1, Red},
				{7, 7, Orange},
				{3, 4, Blue},
			},
		},
		{
			name:       "2-bit all positions in 4x2",
			width:      4,
			height:     2,
			colorDepth: 2,
			testCases: []struct {
				x     int
				y     int
				color Color
			}{
				{0, 0, Color(0)},
				{1, 0, Color(1)},
				{2, 0, Color(2)},
				{3, 0, Color(3)},
				{0, 1, Color(3)},
				{1, 1, Color(2)},
				{2, 1, Color(1)},
				{3, 1, Color(0)},
			},
		},
		{
			name:       "1-bit checkerboard 8x2",
			width:      8,
			height:     2,
			colorDepth: 1,
			testCases: []struct {
				x     int
				y     int
				color Color
			}{
				{0, 0, Black},
				{1, 0, White},
				{2, 0, Black},
				{3, 0, White},
				{4, 0, Black},
				{5, 0, White},
				{6, 0, Black},
				{7, 0, White},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := NewFramebuffer(tt.width, tt.height, tt.colorDepth)
			
			// Set all test pixels
			for _, tc := range tt.testCases {
				fb.SetPixel(tc.x, tc.y, tc.color)
			}
			
			// Verify all test pixels
			for _, tc := range tt.testCases {
				got := fb.GetPixel(tc.x, tc.y)
				if got != tc.color {
					t.Errorf("GetPixel(%d, %d) = %d, want %d", tc.x, tc.y, got, tc.color)
				}
			}
		})
	}
}

// TestPixelIndependence verifies that setting one pixel doesn't affect its neighbors
func TestPixelIndependence(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		colorDepth int
	}{
		{"4-bit 8x8", 8, 8, 4},
		{"2-bit 8x8", 8, 8, 2},
		{"1-bit 16x16", 16, 16, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := NewFramebuffer(tt.width, tt.height, tt.colorDepth)
			
			maxColor := (1 << tt.colorDepth) - 1
			
			// Set every other pixel to a non-zero value
			for y := 0; y < tt.height; y++ {
				for x := 0; x < tt.width; x++ {
					if (x+y)%2 == 0 {
						fb.SetPixel(x, y, Color(maxColor))
					}
				}
			}
			
			// Verify the pattern
			for y := 0; y < tt.height; y++ {
				for x := 0; x < tt.width; x++ {
					got := fb.GetPixel(x, y)
					var want Color
					if (x+y)%2 == 0 {
						want = Color(maxColor)
					} else {
						want = Black
					}
					if got != want {
						t.Errorf("GetPixel(%d, %d) = %d, want %d", x, y, got, want)
					}
				}
			}
		})
	}
}

// TestAllColorValues tests that all possible color values can be stored and retrieved
func TestAllColorValues(t *testing.T) {
	tests := []struct {
		name       string
		colorDepth int
	}{
		{"1-bit", 1},
		{"2-bit", 2},
		{"4-bit", 4},
		{"8-bit", 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxColor := 1 << tt.colorDepth
			fb := NewFramebuffer(maxColor, 1, tt.colorDepth)
			
			// Set each possible color value
			for i := 0; i < maxColor; i++ {
				fb.SetPixel(i, 0, Color(i))
			}
			
			// Verify each color value
			for i := 0; i < maxColor; i++ {
				got := fb.GetPixel(i, 0)
				want := Color(i)
				if got != want {
					t.Errorf("GetPixel(%d, 0) = %d, want %d", i, got, want)
				}
			}
		})
	}
}

// TestBoundsChecking tests out-of-bounds access handling
func TestBoundsChecking(t *testing.T) {
	fb := NewFramebuffer(8, 8, 4)
	
	// Set a known pixel
	fb.SetPixel(0, 0, White)
	
	// Test out-of-bounds SetPixel (should not panic)
	fb.SetPixel(-1, 0, Red)
	fb.SetPixel(0, -1, Red)
	fb.SetPixel(8, 0, Red)
	fb.SetPixel(0, 8, Red)
	fb.SetPixel(100, 100, Red)
	
	// Verify the known pixel wasn't affected
	if got := fb.GetPixel(0, 0); got != White {
		t.Errorf("GetPixel(0, 0) = %d, want %d (pixel was affected by out-of-bounds write)", got, White)
	}
	
	// Test out-of-bounds GetPixel (should return Black)
	tests := []struct {
		x, y int
	}{
		{-1, 0},
		{0, -1},
		{8, 0},
		{0, 8},
		{100, 100},
	}
	
	for _, tc := range tests {
		got := fb.GetPixel(tc.x, tc.y)
		if got != Black {
			t.Errorf("GetPixel(%d, %d) = %d, want %d (out-of-bounds should return Black)", tc.x, tc.y, got, Black)
		}
	}
}

// TestPackedFormatCorrectness tests the bit packing for different color depths
func TestPackedFormatCorrectness(t *testing.T) {
	t.Run("4-bit two pixels per byte", func(t *testing.T) {
		fb := NewFramebuffer(2, 1, 4)
		
		// Set two pixels that share a byte
		fb.SetPixel(0, 0, Color(0x5)) // Lower nibble
		fb.SetPixel(1, 0, Color(0xA)) // Upper nibble
		
		// Check the underlying buffer
		buf := fb.Buffer()
		if len(buf) != 1 {
			t.Fatalf("Buffer length = %d, want 1", len(buf))
		}
		
		// Verify the byte contains both pixels correctly packed
		// Lower nibble (pixel 0) should be 0x5, upper nibble (pixel 1) should be 0xA
		expected := byte(0xA5) // 0xA in upper nibble, 0x5 in lower nibble
		if buf[0] != expected {
			t.Errorf("Buffer[0] = 0x%02X, want 0x%02X", buf[0], expected)
		}
		
		// Verify GetPixel returns correct values
		if got := fb.GetPixel(0, 0); got != Color(0x5) {
			t.Errorf("GetPixel(0, 0) = %d, want 5", got)
		}
		if got := fb.GetPixel(1, 0); got != Color(0xA) {
			t.Errorf("GetPixel(1, 0) = %d, want 10", got)
		}
	})
	
	t.Run("2-bit four pixels per byte", func(t *testing.T) {
		fb := NewFramebuffer(4, 1, 2)
		
		// Set four pixels that share a byte
		fb.SetPixel(0, 0, Color(0))
		fb.SetPixel(1, 0, Color(1))
		fb.SetPixel(2, 0, Color(2))
		fb.SetPixel(3, 0, Color(3))
		
		// Verify GetPixel returns correct values
		for i := 0; i < 4; i++ {
			if got := fb.GetPixel(i, 0); got != Color(i) {
				t.Errorf("GetPixel(%d, 0) = %d, want %d", i, got, i)
			}
		}
	})
	
	t.Run("1-bit eight pixels per byte", func(t *testing.T) {
		fb := NewFramebuffer(8, 1, 1)
		
		// Set an alternating pattern
		for i := 0; i < 8; i++ {
			fb.SetPixel(i, 0, Color(i%2))
		}
		
		// Verify GetPixel returns correct values
		for i := 0; i < 8; i++ {
			want := Color(i % 2)
			if got := fb.GetPixel(i, 0); got != want {
				t.Errorf("GetPixel(%d, 0) = %d, want %d", i, got, want)
			}
		}
	})
}

// TestSequentialPixelWrites tests writing pixels sequentially across byte boundaries
func TestSequentialPixelWrites(t *testing.T) {
	t.Run("4-bit sequential", func(t *testing.T) {
		width := 10
		fb := NewFramebuffer(width, 1, 4)
		
		// Write sequential values
		for i := 0; i < width; i++ {
			fb.SetPixel(i, 0, Color(i%16))
		}
		
		// Verify all values
		for i := 0; i < width; i++ {
			want := Color(i % 16)
			if got := fb.GetPixel(i, 0); got != want {
				t.Errorf("GetPixel(%d, 0) = %d, want %d", i, got, want)
			}
		}
	})
}

// TestOverwritePixel tests that overwriting a pixel works correctly
func TestOverwritePixel(t *testing.T) {
	fb := NewFramebuffer(4, 4, 4)
	
	// Set initial value
	fb.SetPixel(2, 2, Red)
	if got := fb.GetPixel(2, 2); got != Red {
		t.Fatalf("Initial SetPixel failed: got %d, want %d", got, Red)
	}
	
	// Overwrite with different value
	fb.SetPixel(2, 2, Yellow)
	if got := fb.GetPixel(2, 2); got != Yellow {
		t.Errorf("Overwrite failed: got %d, want %d", got, Yellow)
	}
	
	// Overwrite with zero
	fb.SetPixel(2, 2, Black)
	if got := fb.GetPixel(2, 2); got != Black {
		t.Errorf("Overwrite with zero failed: got %d, want %d", got, Black)
	}
}

// TestMultiRowFramebuffer tests pixel operations across multiple rows
func TestMultiRowFramebuffer(t *testing.T) {
	fb := NewFramebuffer(8, 4, 4)
	
	// Set a diagonal pattern
	for i := 0; i < 4; i++ {
		fb.SetPixel(i*2, i, Color(i+1))
	}
	
	// Verify the pattern
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			var want Color
			if x == y*2 {
				want = Color(y + 1)
			} else {
				want = Black
			}
			if got := fb.GetPixel(x, y); got != want {
				t.Errorf("GetPixel(%d, %d) = %d, want %d", x, y, got, want)
			}
		}
	}
}
