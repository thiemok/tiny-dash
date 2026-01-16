package ssd1608

// SSD1608 controller command constants
// Based on Pimoroni's Python implementation
const (
	cmdDriverControl    = 0x01 // Driver Control
	cmdGateVoltage      = 0x03 // Gate Voltage
	cmdSourceVoltage    = 0x04 // Source Voltage
	cmdDisplayControl   = 0x07 // Display Control
	cmdNonOverlap       = 0x0B // Non-Overlap
	cmdBoosterSoftStart = 0x0C // Booster Soft Start
	cmdGateScanStart    = 0x0F // Gate Scan Start
	cmdDeepSleep        = 0x10 // Deep Sleep
	cmdDataMode         = 0x11 // Data Mode
	cmdSwReset          = 0x12 // Software Reset
	cmdTempWrite        = 0x1A // Temperature Write
	cmdTempRead         = 0x1B // Temperature Read
	cmdTempControl      = 0x1C // Temperature Control
	cmdTempLoad         = 0x1D // Temperature Load
	cmdMasterActivate   = 0x20 // Master Activate
	cmdDispCtrl1        = 0x21 // Display Control 1
	cmdDispCtrl2        = 0x22 // Display Control 2
	cmdWriteRAM         = 0x24 // Write RAM (inky.Black/inky.White)
	cmdReadRAM          = 0x25 // Read RAM
	cmdWriteAltRAM      = 0x26 // Write Alternate RAM (inky.Red/inky.Yellow)
	cmdVCOMSense        = 0x28 // VCOM Sense
	cmdVCOMDuration     = 0x29 // VCOM Duration
	cmdWriteVCOM        = 0x2C // Write VCOM
	cmdReadOTP          = 0x2D // Read OTP
	cmdWriteLUT         = 0x32 // Write LUT Register
	cmdWriteDummy       = 0x3A // Write Dummy Line Period
	cmdWriteGateLine    = 0x3B // Write Gate Line Width
	cmdWriteBorder      = 0x3C // Write Border Waveform
	cmdSetRAMXPos       = 0x44 // Set RAM X Address Start/End Position
	cmdSetRAMYPos       = 0x45 // Set RAM Y Address Start/End Position
	cmdSetRAMXCount     = 0x4E // Set RAM X Address Counter
	cmdSetRAMYCount     = 0x4F // Set RAM Y Address Counter
	cmdNOP              = 0xFF // No Operation
)
