package uc8159

// UC8159 controller command set
// Used by 7-color displays (inky.Black, inky.White, inky.Green, inky.Blue, inky.Red, inky.Yellow, inky.Orange)
// 4-bit per pixel format (2 pixels per byte)

const (
	cmdPSR   = 0x00 // Panel Setting Register
	cmdPWR   = 0x01 // Power Setting
	cmdPOF   = 0x02 // Power Off
	cmdPFS   = 0x03 // Power Off Sequence Setting
	cmdPON   = 0x04 // Power On
	cmdBTST  = 0x06 // Booster Soft Start
	cmdDSLP  = 0x07 // Deep Sleep
	cmdDTM1  = 0x10 // Data Transmission Mode 1
	cmdDSP   = 0x11 // Data Stop
	cmdDRF   = 0x12 // Display Refresh
	cmdIPC   = 0x13 // Image Process Command
	cmdPLL   = 0x30 // PLL Control
	cmdTSC   = 0x40 // Temperature Sensor Calibration
	cmdTSE   = 0x41 // Temperature Sensor Enable
	cmdTSW   = 0x42 // Temperature Sensor Write
	cmdTSR   = 0x43 // Temperature Sensor Read
	cmdCDI   = 0x50 // VCOM and Data Interval Setting
	cmdLPD   = 0x51 // Low Power Detection
	cmdTCON  = 0x60 // TCON Setting
	cmdTRES  = 0x61 // Resolution Setting
	cmdDAM   = 0x65 // Data Access Mode
	cmdREV   = 0x70 // Revision
	cmdFLG   = 0x71 // Get Status
	cmdAMV   = 0x80 // Auto Measure VCOM
	cmdVV    = 0x81 // VCOM Value
	cmdVDCS  = 0x82 // VCOM DC Setting
	cmdPWS   = 0xE3 // Power Saving
	cmdTSSET = 0xE5 // Temperature Sensor Setting
)
