package el133uf1

// EL133UF1 controller command set
// Used by 1600x1200 6-color display (inky.Black, inky.White, inky.Yellow, inky.Red, inky.Blue, inky.Green)
// 4-bit per pixel format (2 pixels per byte)
// NOTE: Uses dual chip select pins (CS0 and CS1) for left/right display halves

const (
	cmdPSR             = 0x00 // Panel Setting Register
	cmdPWR             = 0x01 // Power Setting
	cmdPOF             = 0x02 // Power Off
	cmdPON             = 0x04 // Power On
	cmdBTST_N          = 0x05 // Booster Soft Start Negative
	cmdBTST_P          = 0x06 // Booster Soft Start Positive
	cmdDTM             = 0x10 // Data Transmission
	cmdDRF             = 0x12 // Display Refresh
	cmdPLL             = 0x30 // PLL Control
	cmdTSC             = 0x40 // Temperature Sensor Calibration
	cmdTSE             = 0x41 // Temperature Sensor Enable
	cmdTSW             = 0x42 // Temperature Sensor Write
	cmdTSR             = 0x43 // Temperature Sensor Read
	cmdCDI             = 0x50 // VCOM and Data Interval Setting
	cmdLPD             = 0x51 // Low Power Detection
	cmdTCON            = 0x60 // TCON Setting
	cmdTRES            = 0x61 // Resolution Setting
	cmdDAM             = 0x65 // Data Access Mode
	cmdREV             = 0x70 // Revision
	cmdFLG             = 0x71 // Get Status
	cmdANTM            = 0x74 // Analog Temperature
	cmdAMV             = 0x80 // Auto Measure VCOM
	cmdVV              = 0x81 // VCOM Value
	cmdVDCS            = 0x82 // VCOM DC Setting
	cmdPTLW            = 0x83 // Partial Window
	cmdAGID            = 0x86 // Auto Gate ID
	cmdBUCK_BOOST_VDDN = 0xB0 // Buck Boost VDDN
	cmdTFT_VCOM_POWER  = 0xB1 // TFT VCOM Power
	cmdEN_BUF          = 0xB6 // Enable Buffer
	cmdBOOST_VDDP_EN   = 0xB7 // Boost VDDP Enable
	cmdCCSET           = 0xE0 // Cascade Setting
	cmdPWS             = 0xE3 // Power Saving
	cmdTSSET           = 0xE5 // Temperature Sensor Setting
	cmdCMD66           = 0xF0 // Command 66 (initialization)
)

// Chip select flags for dual CS displays
const (
	csNone = 0b00
	cs0Sel = 0b01
	cs1Sel = 0b10
	csBoth = cs0Sel | cs1Sel
)
