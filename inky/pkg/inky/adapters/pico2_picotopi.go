package adapters

import (
	"machine"

	"github.com/thiemok/tiny-dash/inky/pkg/inky"
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
		SDA:       machine.I2C1_SDA_PIN,
		SCL:       machine.I2C1_SCL_PIN,
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

	return &inky.InkyConfig{
		SPI:  &Pico2SPI{bus: spi},
		I2C:  &Pico2I2C{bus: i2c},
		CS:   cs,
		DC:   dc,
		RST:  rst,
		BUSY: busy,
	}, nil
}
