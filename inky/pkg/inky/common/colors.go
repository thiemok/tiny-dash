package common

// Color represents a color value for Inky e-ink displays
// Different display types support different color subsets
type Color byte

const (
	Black  Color = 0
	White  Color = 1
	Yellow Color = 2
	Red    Color = 3
	Orange Color = 4 // For UC8159 7-color displays
	Blue   Color = 5
	Green  Color = 6
	Clean  Color = 7 // For UC8159 cleaning/clear mode
)

// ColorRGB provides RGB values for each color (for reference/conversion)
var ColorRGB = map[Color][3]byte{
	Black:  {0, 0, 0},
	White:  {255, 255, 255},
	Yellow: {255, 255, 0},
	Red:    {255, 0, 0},
	Orange: {255, 140, 0},
	Blue:   {0, 0, 255},
	Green:  {0, 255, 0},
	Clean:  {255, 255, 255}, // Clean is typically white
}

// String returns the color name
func (c Color) String() string {
	switch c {
	case Black:
		return "Black"
	case White:
		return "White"
	case Yellow:
		return "Yellow"
	case Red:
		return "Red"
	case Orange:
		return "Orange"
	case Blue:
		return "Blue"
	case Green:
		return "Green"
	case Clean:
		return "Clean"
	default:
		return "Unknown"
	}
}
