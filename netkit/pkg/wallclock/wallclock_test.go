package wallclock

import (
	"testing"
	"time"
)

var floor = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

type fakeMono struct{ ns int64 }

func (m *fakeMono) now() int64 { return m.ns }

func TestUnsynced(t *testing.T) {
	m := &fakeMono{ns: 5 * int64(time.Second)}
	c := New(m.now, floor)
	if c.Synced() {
		t.Fatal("new clock reports synced")
	}
	if now, ok := c.Now(); ok || !now.IsZero() {
		t.Errorf("Now() = %v, %v; want zero time, false", now, ok)
	}
	if ns, ok := c.UnixNano(); ok || ns != 0 {
		t.Errorf("UnixNano() = %d, %v; want 0, false", ns, ok)
	}
	if ns := c.Nanotime(); ns != 0 {
		t.Errorf("Nanotime() = %d, want 0", ns)
	}
	if got := c.Floor(); !got.Equal(floor) {
		t.Errorf("Floor() = %v, want %v", got, floor)
	}
}

func TestSetAndAdvance(t *testing.T) {
	m := &fakeMono{ns: 5 * int64(time.Second)}
	c := New(m.now, floor)
	wall := floor.Add(3 * time.Hour)
	step, err := c.Set(wall, m.ns)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Duration(wall.UnixNano() - m.ns); step != want {
		t.Errorf("first step = %v, want %v", step, want)
	}
	if !c.Synced() {
		t.Fatal("clock not synced after Set")
	}

	m.ns += int64(90 * time.Second)
	now, ok := c.Now()
	if want := wall.Add(90 * time.Second); !ok || !now.Equal(want) || now.Location() != time.UTC {
		t.Errorf("Now() = %v, %v; want %v, true", now, ok, want)
	}
	if ns := c.Nanotime(); ns != now.UnixNano() {
		t.Errorf("Nanotime() = %d, want %d", ns, now.UnixNano())
	}
}

func TestResyncStep(t *testing.T) {
	m := &fakeMono{}
	c := New(m.now, floor)
	wall := floor.Add(time.Hour)
	if _, err := c.Set(wall, m.ns); err != nil {
		t.Fatal(err)
	}
	m.ns += int64(12 * time.Hour)
	// The local oscillator ran 1.5 s slow over 12 h.
	step, err := c.Set(wall.Add(12*time.Hour+1500*time.Millisecond), m.ns)
	if err != nil {
		t.Fatal(err)
	}
	if step != 1500*time.Millisecond {
		t.Errorf("step = %v, want 1.5s", step)
	}
	step, _ = c.Set(wall.Add(12*time.Hour), m.ns)
	if step != -1500*time.Millisecond {
		t.Errorf("step back = %v, want -1.5s", step)
	}
}

func TestSetRefusesTimeBeforeFloor(t *testing.T) {
	m := &fakeMono{ns: 1}
	c := New(m.now, floor)
	if _, err := c.Set(floor.Add(-time.Second), m.ns); err != ErrBeforeFloor {
		t.Fatalf("Set before floor: err = %v, want ErrBeforeFloor", err)
	}
	if c.Synced() {
		t.Fatal("refused Set synced the clock")
	}
	if _, err := c.Set(floor, m.ns); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Set(time.Unix(0, 0), m.ns); err != ErrBeforeFloor {
		t.Fatalf("Set to 1970: err = %v, want ErrBeforeFloor", err)
	}
	if ns, _ := c.UnixNano(); ns != floor.UnixNano() {
		t.Errorf("refused Set moved the clock to %d", ns)
	}
}

func TestReadsDoNotAllocate(t *testing.T) {
	m := &fakeMono{}
	c := New(m.now, floor)
	unsynced := New(m.now, floor)
	wall := floor.Add(time.Hour)
	allocs := testing.AllocsPerRun(100, func() {
		m.ns++
		unsynced.Now()
		unsynced.Nanotime()
		unsynced.Set(floor.Add(-time.Second), m.ns)
		c.Now()
		c.Nanotime()
		c.Set(wall, m.ns)
		c.Synced()
	})
	if allocs != 0 {
		t.Errorf("clock allocated %v times per run", allocs)
	}
}
