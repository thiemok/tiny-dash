package common

import "testing"

type nopPin struct{}

func (nopPin) Configure(PinMode) error { return nil }
func (nopPin) Set(bool)                {}
func (nopPin) Get() bool               { return true }

type recordSPI struct{ cmds []byte }

func (s *recordSPI) Tx(w, _ []byte) error {
	if len(w) == 1 {
		s.cmds = append(s.cmds, w[0])
	}
	return nil
}

func TestSendCommandDoesNotAllocate(t *testing.T) {
	spi := &recordSPI{cmds: make([]byte, 0, 512)}
	data := []byte{0x6F, 0x1F}
	var cmd byte
	allocs := testing.AllocsPerRun(100, func() {
		cmd++
		SendCommand(spi, nopPin{}, nopPin{}, cmd, data)
	})
	if allocs != 0 {
		t.Errorf("SendCommand allocated %v times per run, want 0", allocs)
	}
	if got := spi.cmds[len(spi.cmds)-1]; got != cmd {
		t.Errorf("last command byte = %#x, want %#x", got, cmd)
	}
}

func TestCommandBytesIsIdentity(t *testing.T) {
	if len(commandBytes) != 256 {
		t.Fatalf("len(commandBytes) = %d, want 256", len(commandBytes))
	}
	for c := 0; c < 256; c++ {
		if commandBytes[c] != byte(c) {
			t.Fatalf("commandBytes[%#x] = %#x", c, commandBytes[c])
		}
	}
}
