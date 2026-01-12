package inky

import "time"

// reset performs a hardware reset sequence on the display
// This is a common operation used by all display types
func reset(rstPin Pin) {
	rstPin.Set(false)
	time.Sleep(30 * time.Millisecond)
	rstPin.Set(true)
	time.Sleep(30 * time.Millisecond)
}

// busyWait waits for the busy pin to go high (ready state)
// Returns true if ready, false if timeout
// This is a common operation used by all display types
func busyWait(busyPin Pin, timeoutSeconds int) bool {
	// If BUSY pin is already high, display is ready
	if busyPin.Get() {
		return true
	}

	// Wait for BUSY to go high
	maxIterations := timeoutSeconds * 10 // Check every 100ms
	for i := 0; i < maxIterations; i++ {
		if busyPin.Get() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}

	return false // Timeout
}

// sendCommand sends a command with optional data to the display via SPI
// This is a common operation used by all display types
func sendCommand(spi SPI, csPin, dcPin Pin, command byte, data []byte) {
	csPin.Set(false)
	dcPin.Set(false)
	time.Sleep(300 * time.Microsecond)
	spi.Tx([]byte{command}, nil)

	if data != nil && len(data) > 0 {
		dcPin.Set(true)
		spi.Tx(data, nil)
	}

	csPin.Set(true)
	dcPin.Set(false)
}
