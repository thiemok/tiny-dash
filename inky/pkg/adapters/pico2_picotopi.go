package adapters

import (
	"machine"

	inky "github.com/thiemok/tiny-dash/inky/pkg/inky/common"
)

// Pin assignments for Pico 2 W with Hard Stuff Pico-to-Pi adapter
// Raspberry Pi HAT pins mapped to Pico GPIO through adapter
// Based on Pimoroni library expectations: RPi GPIO 8(CE0), 17, 22, 27
const (
	pinCS   = machine.GP8          // Chip Select - RPi GPIO 8 (CE0) → GP8
	pinDC   = machine.GP22         // Data/Command - RPi GPIO 22 → GP22
	pinRST  = machine.GP27         // Reset - RPi GPIO 27 → GP27
	pinBUSY = machine.GP17         // Busy signal - RPi GPIO 17 → GP17
	pinSCK  = machine.SPI1_SCK_PIN // SPI CLK - RPi GPIO 11
	pinSDO  = machine.SPI1_SDO_PIN // SPI MOSI - RPi GPIO 10
	pinSDI  = machine.SPI1_SDI_PIN // unused, but required for SPI configuration
	pinSDA  = machine.I2C1_SDA_PIN // I2C SDA - RPi GPIO 2 -> GP3
	pinSCL  = machine.I2C1_SCL_PIN // I2C SCL - RPi GPIO 3 -> GP2

	// Button and LED pins (Pimoroni Inky Impression standard)
	pinButtonA = machine.GP15 // Button A - RPi GPIO 5 → GP15
	pinButtonB = machine.GP6  // Button B - RPi GPIO 6 → GP6
	pinButtonC = machine.GP16 // Button C - RPi GPIO 16 → GP16
	pinButtonD = machine.GP1  // Button D - RPi GPIO 24 → GP24
	pinLED     = machine.GP13 // Status LED - RPi GPIO 13 → GP13 (verified from Pimoroni examples)

	spiFrequency = 1_000_000 // 1 MHz (matching Python implementation)
	spiMode      = 0         // Mode 0 (CPOL=0, CPHA=0)

	i2cFrequency = 100_000 // 100 kHz - standard I2C speed
)

// Pico2Pin wraps machine.Pin to implement the inky.Pin interface
type Pico2Pin struct {
	pin machine.Pin
}

func (p *Pico2Pin) Configure(mode inky.PinMode) error {
	switch mode {
	case inky.PinInput:
		p.pin.Configure(machine.PinConfig{Mode: machine.PinInput})
	case inky.PinOutput:
		p.pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
	return nil
}

func (p *Pico2Pin) Set(high bool) {
	p.pin.Set(high)
}

func (p *Pico2Pin) Get() bool {
	return p.pin.Get()
}

// Pico2I2C wraps machine.I2C to implement the inky.I2C interface
type Pico2I2C struct {
	bus *machine.I2C
}

func (i *Pico2I2C) Tx(addr uint16, w, r []byte) error {
	return i.bus.Tx(addr, w, r)
}

// Pico2SPI wraps machine.SPI to implement the inky.SPI interface
type Pico2SPI struct {
	bus *machine.SPI
}

func (s *Pico2SPI) Tx(w, r []byte) error {
	return s.bus.Tx(w, r)
}

// NewPico2PicoToPiHardware creates and configures hardware for Pico 2 W
// with Hard Stuff Pico-to-Pi adapter
// This automatically configures SPI1, I2C1, and all required GPIO pins
func NewPico2PicoToPiHardware() (*inky.InkyConfig, error) {
	// Configure SPI1
	spi := machine.SPI1
	err := spi.Configure(machine.SPIConfig{
		Frequency: spiFrequency,
		Mode:      spiMode,
		SCK:       pinSCK,
		SDO:       pinSDO,
		SDI:       pinSDI,
	})
	if err != nil {
		return nil, err
	}

	// Configure I2C1
	i2c := machine.I2C1
	err = i2c.Configure(machine.I2CConfig{
		Frequency: i2cFrequency,
		SDA:       pinSDA,
		SCL:       pinSCL,
	})
	if err != nil {
		return nil, err
	}

	// Create pin wrappers
	cs := &Pico2Pin{pin: pinCS}
	dc := &Pico2Pin{pin: pinDC}
	rst := &Pico2Pin{pin: pinRST}
	busy := &Pico2Pin{pin: pinBUSY}

	// Configure pins - displays will configure them as needed
	// But set initial states for safety
	cs.Configure(inky.PinOutput)
	cs.Set(true) // CS idle high

	dc.Configure(inky.PinOutput)
	dc.Set(false) // DC default low

	rst.Configure(inky.PinOutput)
	rst.Set(true) // RST idle high

	busy.Configure(inky.PinInput)

	// Create button and LED pin wrappers (always configured)
	buttonA := &Pico2Pin{pin: pinButtonA}
	buttonB := &Pico2Pin{pin: pinButtonB}
	buttonC := &Pico2Pin{pin: pinButtonC}
	buttonD := &Pico2Pin{pin: pinButtonD}
	led := &Pico2Pin{pin: pinLED}

	return &inky.InkyConfig{
		SPI:  &Pico2SPI{bus: spi},
		I2C:  &Pico2I2C{bus: i2c},
		CS:   cs,
		DC:   dc,
		RST:  rst,
		BUSY: busy,

		// Optional features - displays can use these if they support buttons/LED
		ButtonPins: []inky.Pin{buttonA, buttonB, buttonC, buttonD},
		LEDPin:     led,
	}, nil
}
