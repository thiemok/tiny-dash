package common

import (
	"testing"
	"time"
)

// irqPin is an active-low button pin whose level the test sets, firing the
// change interrupt like the hardware would.
type irqPin struct {
	level bool
	cb    func()
}

func (p *irqPin) Configure(PinMode) error { return nil }
func (p *irqPin) Set(bool)                {}
func (p *irqPin) Get() bool               { return p.level }
func (p *irqPin) SetChangeInterrupt(cb func()) error {
	p.cb = cb
	return nil
}

// edge sets the level at t ms on the fake clock and fires the interrupt.
func (p *irqPin) edge(clock *int64, t int64, level bool) {
	*clock = t * int64(time.Millisecond)
	p.level = level
	p.cb()
}

func TestInterruptPressesAreLatchedAndDebounced(t *testing.T) {
	var clock int64
	defer func(f func() int64) { nowNano = f }(nowNano)
	nowNano = func() int64 { return clock }

	pin := &irqPin{level: true}
	bc, err := NewButtonController([]Pin{pin, nopPin{}}, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := bc.EnableInterrupts(); n != 1 || err != nil {
		t.Fatalf("EnableInterrupts = %d, %v; want 1 interrupt-driven button", n, err)
	}

	// A press with contact bounce while nothing polls, as during a refresh.
	pin.edge(&clock, 1000, false)
	pin.edge(&clock, 1002, true)
	pin.edge(&clock, 1003, false)
	// The release, also bouncing.
	pin.edge(&clock, 1200, true)
	pin.edge(&clock, 1201, false)
	pin.edge(&clock, 1202, true)

	if !bc.WasPressed(0) {
		t.Fatal("press during no polling was not latched")
	}
	if bc.WasPressed(0) {
		t.Fatal("bounces counted as a second press")
	}

	// Polling must not report the same press again.
	pin.level = false
	bc.Poll()
	time.Sleep(60 * time.Millisecond)
	bc.Poll()
	if bc.WasPressed(0) {
		t.Fatal("Poll reported a press of an interrupt-driven button")
	}

	// A later, separate press counts.
	pin.edge(&clock, 2000, true)
	pin.edge(&clock, 2500, false)
	if !bc.WasPressed(0) {
		t.Fatal("second press was not latched")
	}
}
