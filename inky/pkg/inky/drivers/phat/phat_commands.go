package phat

// Common e-ink display command constants
// Based on Pimoroni's Python implementation
const (
	CmdPSR   = 0x00 // Panel Setting Register
	CmdPWR   = 0x01 // Power Setting / Gate Setting
	CmdPOF   = 0x02 // Power Off
	CmdPOFS  = 0x03 // Power Off Sequence Setting / Gate Driving Voltage
	CmdPON   = 0x04 // Power On / Source Driving Voltage
	CmdBTST1 = 0x05 // Booster Soft Start 1
	CmdBTST2 = 0x06 // Booster Soft Start 2
	CmdDSLP  = 0x07 // Deep Sleep
	CmdBTST3 = 0x08 // Booster Soft Start 3
	CmdDTM1  = 0x10 // Data Start Transmission 1 / Enter Deep Sleep
	CmdDSP   = 0x11 // Data Stop / Data Entry Mode Setting
	CmdDRF   = 0x12 // Display Refresh / Soft Reset
	CmdTRIGR = 0x20 // Trigger Display Update (base Inky class)
	CmdUPDSQ = 0x22 // Display Update Sequence (base Inky class)
	CmdRAMBW = 0x24 // Write RAM (Black/White buffer)
	CmdRAMRY = 0x26 // Write RAM (Red/Yellow buffer)
	CmdVCOM  = 0x2C // VCOM Register
	CmdPLL   = 0x30 // PLL Control
	CmdWLUT  = 0x32 // Write LUT Register
	CmdDLP   = 0x3A // Dummy Line Period
	CmdGLW   = 0x3B // Gate Line Width
	CmdBDR   = 0x3C // Border Waveform Control
	CmdRAMXS = 0x44 // Set RAM X Address Start/End Position
	CmdRAMYS = 0x45 // Set RAM Y Address Start/End Position
	CmdRAMXP = 0x4E // Set RAM X Address Counter
	CmdRAMYP = 0x4F // Set RAM Y Address Counter
	CmdCDI   = 0x50 // VCOM and Data Interval Setting
	CmdTCON  = 0x60 // TCON Setting
	CmdTRES  = 0x61 // Resolution Setting
	CmdREV   = 0x70 // Revision
	CmdABC   = 0x74 // Set Analog Block Control
	CmdDBC   = 0x7E // Set Digital Block Control
	CmdVDCS  = 0x82 // VCOM DC Setting
	CmdPWS   = 0xE3 // Power Saving
)
