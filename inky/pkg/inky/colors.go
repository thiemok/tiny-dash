package inky

// Color represents a 6-color palette value for the Inky Spectra 6 display
type Color byte

const (
	Black  Color = 0
	White  Color = 1
	Yellow Color = 2
	Red    Color = 3
	Blue   Color = 5
	Green  Color = 6
)

// ColorRGB provides RGB values for each color (for reference/conversion)
var ColorRGB = map[Color][3]byte{
	Black:  {0, 0, 0},
	White:  {255, 255, 255},
	Yellow: {255, 255, 0},
	Red:    {255, 0, 0},
	Blue:   {0, 0, 255},
	Green:  {0, 255, 0},
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
	case Blue:
		return "Blue"
	case Green:
		return "Green"
	default:
		return "Unknown"
	}
}
