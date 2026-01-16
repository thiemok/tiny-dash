package ssd1683

// SSD1683 controller command constants
// Based on Pimoroni's Python implementation
// Note: SSD1683 commands are nearly identical to SSD1608
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
	cmdTempControl      = 0x18 // Temperature Control
	cmdTempWrite        = 0x1A // Temperature Write / Temperature Load
	cmdTempRead         = 0x1B // Temperature Read
	cmdMasterActivate   = 0x20 // Master Activate
	cmdDispCtrl1        = 0x21 // Display Control 1
	cmdDispCtrl2        = 0x22 // Display Control 2
	cmdWriteRAM         = 0x24 // Write RAM (inky.Black/inky.White)
	cmdReadRAM          = 0x25 // Read RAM
	cmdWriteAltRAM      = 0x26 // Write Alternate RAM (inky.Red/inky.Yellow)
	cmdVCOMSense        = 0x2B // VCOM Sense
	cmdVCOMDuration     = 0x2C // VCOM Duration / Write VCOM
	cmdWriteVCOM        = 0x2C // Write VCOM (same as VCOMDuration)
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
