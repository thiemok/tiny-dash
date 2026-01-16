package ac073tc1a

// AC073TC1A controller command set
// Used by 800x480 7-color display (inky.Black, inky.White, inky.Green, inky.Blue, inky.Red, inky.Yellow, inky.Orange)
// 4-bit per pixel format (2 pixels per byte)

const (
	cmdPSR    = 0x00 // Panel Setting Register
	cmdPWR    = 0x01 // Power Setting
	cmdPOF    = 0x02 // Power Off
	cmdPOFS   = 0x03 // Power Off Sequence Setting
	cmdPON    = 0x04 // Power On
	cmdBTST1  = 0x05 // Booster Soft Start 1
	cmdBTST2  = 0x06 // Booster Soft Start 2
	cmdDSLP   = 0x07 // Deep Sleep
	cmdBTST3  = 0x08 // Booster Soft Start 3
	cmdDTM    = 0x10 // Data Transmission
	cmdDSP    = 0x11 // Data Stop
	cmdDRF    = 0x12 // Display Refresh
	cmdIPC    = 0x13 // Image Process Command
	cmdPLL    = 0x30 // PLL Control
	cmdTSC    = 0x40 // Temperature Sensor Calibration
	cmdTSE    = 0x41 // Temperature Sensor Enable
	cmdTSW    = 0x42 // Temperature Sensor Write
	cmdTSR    = 0x43 // Temperature Sensor Read
	cmdCDI    = 0x50 // VCOM and Data Interval Setting
	cmdLPD    = 0x51 // Low Power Detection
	cmdTCON   = 0x60 // TCON Setting
	cmdTRES   = 0x61 // Resolution Setting
	cmdDAM    = 0x65 // Data Access Mode
	cmdREV    = 0x70 // Revision
	cmdFLG    = 0x71 // Get Status
	cmdAMV    = 0x80 // Auto Measure VCOM
	cmdVV     = 0x81 // VCOM Value
	cmdVDCS   = 0x82 // VCOM DC Setting
	cmdT_VDCS = 0x84 // Temperature VCOM DC Setting
	cmdAGID   = 0x86 // Auto Gate ID
	cmdCMDH   = 0xAA // Command Header
	cmdCCSET  = 0xE0 // Cascade Setting
	cmdPWS    = 0xE3 // Power Saving
	cmdTSSET  = 0xE6 // Temperature Sensor Setting
)
