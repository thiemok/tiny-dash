package common

// PinMode represents pin configuration mode
type PinMode uint8

const (
	PinInput PinMode = iota
	PinOutput
)

// Pin interface for GPIO operations
// Implementations must handle pin configuration and state management
type Pin interface {
	// Configure sets up the pin mode (input or output)
	// Must be called before using Set() or Get()
	Configure(mode PinMode) error

	// Set sets the pin output state (for output pins)
	// high=true sets pin HIGH, high=false sets pin LOW
	Set(high bool)

	// Get reads the pin input state (for input pins)
	// Returns true if pin is HIGH, false if LOW
	Get() bool
}

// I2C interface for EEPROM and other I2C operations
type I2C interface {
	// Tx performs an I2C transaction
	// addr: I2C device address (7-bit or 8-bit depending on implementation)
	// w: data to write (can be nil for read-only operation)
	// r: buffer for data to read (can be nil for write-only operation)
	// Returns error if transaction fails
	Tx(addr uint16, w, r []byte) error
}

// SPI interface for display communication
type SPI interface {
	// Tx performs an SPI transaction
	// w: data to write
	// r: buffer for data to read (typically nil for display operations)
	// Returns error if transaction fails
	Tx(w, r []byte) error
}

// InkyConfig bundles hardware interfaces for display initialization
// All fields except CS1, ButtonPins, and LEDPin are required
type InkyConfig struct {
	SPI  SPI // SPI bus for display communication
	I2C  I2C // I2C bus for EEPROM reading
	CS   Pin // Chip Select pin (primary)
	CS1  Pin // Chip Select pin 1 (optional, only for EL133UF1 dual-CS display)
	DC   Pin // Data/Command pin
	RST  Pin // Reset pin
	BUSY Pin // Busy signal pin (input)

	// Optional features (not all displays support these)
	ButtonPins []Pin // Button pins (optional, for displays with buttons)
	LEDPin     Pin   // LED pin (optional, for displays with status LED)
}
