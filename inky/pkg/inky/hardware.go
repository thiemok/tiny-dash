package inky

import (
	"machine"
	"time"
)

// Pin assignments for Inky Impression 7.3" Spectra 6 via Hard Stuff Pico to Pi adapter
// Raspberry Pi HAT pins mapped to Pico GPIO through adapter
// Based on Pimoroni library expectations: RPi GPIO 8(CE0), 17, 22, 27
const (
	pinCS   = machine.GP8          // Chip Select - RPi GPIO 8 (CE0) → GP8 ✅
	pinDC   = machine.GP22         // Data/Command - RPi GPIO 22 → GP22 ✅
	pinRST  = machine.GP27         // Reset - RPi GPIO 27 → GP27
	pinBUSY = machine.GP17         // Busy signal - RPi GPIO 17 → GP17 ✅
	pinSCK  = machine.SPI1_SCK_PIN // SPI CLK - RPi GPIO 11 ->
	pinSDO  = machine.SPI1_SDO_PIN // SPI MOSI - RPi  GPIO 10 ->

	pinSDI = machine.SPI1_SDI_PIN // unused, but required to be matching for configuration

	spiFrequency = 1_000_000 // 1 MHz (matching Python implementation)
	spiMode      = 0         // Mode 0 (CPOL=0, CPHA=0)
)

// hardwareSPI wraps the TinyGo SPI device
type hardwareSPI struct {
	device *machine.SPI
}

// transfer sends data over SPI
func (h *hardwareSPI) transfer(data []byte) {
	h.device.Tx(data, nil)
}

// hardwareGPIO manages GPIO pins for the display
type hardwareGPIO struct {
	csPin   machine.Pin
	dcPin   machine.Pin
	rstPin  machine.Pin
	busyPin machine.Pin
}

// setCS sets the chip select pin state
func (h *hardwareGPIO) setCS(high bool) {
	h.csPin.Set(high)
}

// setDC sets the data/command pin state
func (h *hardwareGPIO) setDC(high bool) {
	h.dcPin.Set(high)
}

// setRST sets the reset pin state
func (h *hardwareGPIO) setRST(high bool) {
	h.rstPin.Set(high)
}

// readBUSY reads the busy pin state
func (h *hardwareGPIO) readBUSY() bool {
	return h.busyPin.Get()
}

// initSPI initializes SPI with correct configuration
func initSPI() *hardwareSPI {
	spi := machine.SPI1
	err := spi.Configure(machine.SPIConfig{
		Frequency: spiFrequency,
		Mode:      spiMode,
		SCK:       pinSCK,
		SDO:       pinSDO,
		SDI:       pinSDI,
	})
	if err != nil {
		// In TinyGo, Configure typically doesn't return errors
		// but we handle it just in case
		println("Warning: SPI configuration may have failed")
		println(err.Error())
	}

	return &hardwareSPI{device: spi}
}

// initGPIO initializes GPIO pins with correct configuration
func initGPIO() *hardwareGPIO {
	// Configure output pins
	pinCS.Configure(machine.PinConfig{Mode: machine.PinOutput})
	pinDC.Configure(machine.PinConfig{Mode: machine.PinOutput})
	pinRST.Configure(machine.PinConfig{Mode: machine.PinOutput})

	// Configure input pin
	pinBUSY.Configure(machine.PinConfig{Mode: machine.PinInput})

	// Set initial states
	pinCS.High()  // CS idle high
	pinDC.Low()   // DC default low
	pinRST.High() // RST idle high

	return &hardwareGPIO{
		csPin:   pinCS,
		dcPin:   pinDC,
		rstPin:  pinRST,
		busyPin: pinBUSY,
	}
}

// reset performs a hardware reset sequence
func (h *hardwareGPIO) reset() {
	h.setRST(false)
	time.Sleep(30 * time.Millisecond)
	h.setRST(true)
	time.Sleep(30 * time.Millisecond)
}

// busyWait waits for the busy pin to go high (ready state)
// Returns true if ready, false if timeout
func (h *hardwareGPIO) busyWait(timeoutSeconds int) bool {
	// If BUSY pin is already high, display is ready
	if h.readBUSY() {
		return true
	}

	// Wait for BUSY to go high
	maxIterations := timeoutSeconds * 10 // Check every 100ms
	for i := 0; i < maxIterations; i++ {
		if h.readBUSY() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}

	return false // Timeout
}
