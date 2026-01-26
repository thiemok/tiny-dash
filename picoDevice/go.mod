module github.com/thiemok/tiny-dash/picoDevice

go 1.25

require (
	// TODO point back at main
	github.com/soypat/cyw43439 v0.0.0-20260125204952-1d59f013ae39
	github.com/soypat/lneto v0.0.0-20260126151213-6a6097a9ace7
	github.com/thiemok/tiny-dash/inky v0.0.0
	github.com/thiemok/tiny-dash/util v0.0.0
)

require (
	github.com/soypat/seqs v0.0.0-20260125140838-2c1c6b1bd69e // indirect
	github.com/tinygo-org/pio v0.2.0 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
)

replace github.com/thiemok/tiny-dash/inky => ../inky

replace github.com/thiemok/tiny-dash/util => ../util
