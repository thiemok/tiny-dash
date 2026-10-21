module github.com/thiemok/tiny-dash/picoDevice

go 1.26.0

require (
	github.com/soypat/cyw43439 v0.1.2-0.20261015151841-318d5aff4d24
	github.com/soypat/lneto v0.4.1-0.20261020183255-fd69eebbb0c3
	github.com/thiemok/tiny-dash/inky v0.0.0
	github.com/thiemok/tiny-dash/logkit v0.0.0
	github.com/thiemok/tiny-dash/netkit v0.0.0
	github.com/thiemok/tiny-dash/util v0.0.0
)

require (
	github.com/soypat/seqs v0.0.0-20260125140838-2c1c6b1bd69e // indirect
	github.com/tinygo-org/pio v0.3.0 // indirect
	golang.org/x/exp v0.0.0-20260820122028-d6e0b57b1a69 // indirect
)

replace github.com/thiemok/tiny-dash/inky => ../inky

replace github.com/thiemok/tiny-dash/netkit => ../netkit

replace github.com/thiemok/tiny-dash/util => ../util

replace github.com/thiemok/tiny-dash/logkit => ../logkit
