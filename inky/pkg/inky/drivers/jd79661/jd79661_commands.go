package jd79661

// JD79661 controller command set
// Used by 4-color displays (inky.Black, inky.White, inky.Yellow, inky.Red)
// 2-bit per pixel format (4 pixels per byte)

const (
	CmdPSR    = 0x00 // Panel Setting Register
	CmdPWR    = 0x01 // Power Setting
	CmdPOF    = 0x02 // Power Off
	CmdPOFS   = 0x03 // Power Off Sequence Setting
	CmdPON    = 0x04 // Power On
	CmdBTST_P = 0x06 // Booster Soft Start
	CmdDSLP   = 0x07 // Deep Sleep
	CmdDTM    = 0x10 // Data Transmission
	CmdDRF    = 0x12 // Display Refresh
	CmdCDI    = 0x50 // VCOM and Data Interval Setting
	CmdTCON   = 0x60 // TCON Setting
	CmdTRES   = 0x61 // Resolution Setting
	CmdPWS    = 0xE3 // Power Saving
)

// JD79661 address start values
// X_ADDR_START: 0x0080 (128)
// Y_ADDR_START: 0x00FA (250)
const (
	jd79661_X_ADDR_START_H = 0x00
	jd79661_X_ADDR_START_L = 0x80
	jd79661_Y_ADDR_START_H = 0x00
	jd79661_Y_ADDR_START_L = 0xFA
)
