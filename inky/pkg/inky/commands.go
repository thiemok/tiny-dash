package inky

// e640 controller command constants
// Based on Pimoroni's Python implementation
const (
	cmdPSR   = 0x00 // Panel Setting Register
	cmdPWR   = 0x01 // Power Setting
	cmdPOF   = 0x02 // Power Off
	cmdPOFS  = 0x03 // Power Off Sequence Setting
	cmdPON   = 0x04 // Power On
	cmdBTST1 = 0x05 // Booster Soft Start 1
	cmdBTST2 = 0x06 // Booster Soft Start 2
	cmdDSLP  = 0x07 // Deep Sleep
	cmdBTST3 = 0x08 // Booster Soft Start 3
	cmdDTM1  = 0x10 // Data Start Transmission 1
	cmdDSP   = 0x11 // Data Stop
	cmdDRF   = 0x12 // Display Refresh
	cmdPLL   = 0x30 // PLL Control
	cmdCDI   = 0x50 // VCOM and Data Interval Setting
	cmdTCON  = 0x60 // TCON Setting
	cmdTRES  = 0x61 // Resolution Setting
	cmdREV   = 0x70 // Revision
	cmdVDCS  = 0x82 // VCOM DC Setting
	cmdPWS   = 0xE3 // Power Saving
)
