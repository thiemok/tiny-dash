package ntpquery

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/soypat/lneto/ntp"
)

const nonce = 0x0123456789abcdef

var serverTime = time.Date(2026, 10, 20, 12, 0, 0, 250_000_000, time.UTC)

// response is a server answer that received the request at rx and sent
// itself at tx.
func response(t *testing.T, origin uint64, rx, tx time.Time) []byte {
	t.Helper()
	b := make([]byte, Size)
	frm, _ := ntp.NewFrame(b)
	frm.SetFlags(ntp.ModeServer, ntp.Version4, ntp.LeapNoWarning)
	frm.SetStratum(2)
	copy(frm.ReferenceID()[:], "GPS\x00")
	frm.SetOriginTime(ntp.TimestampFromUint64(origin))
	frm.SetReceiveTime(timestamp(t, rx))
	frm.SetTransmitTime(timestamp(t, tx))
	return b
}

func timestamp(t *testing.T, tm time.Time) ntp.Timestamp {
	t.Helper()
	ts, err := ntp.TimestampFromTime(tm)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestAppendRequest(t *testing.T) {
	prefix := []byte("hdr")
	req := AppendRequest(prefix, nonce)
	if len(req) != len(prefix)+Size || string(req[:3]) != "hdr" {
		t.Fatalf("AppendRequest returned %d bytes, want prefix + %d", len(req), Size)
	}
	frm, _ := ntp.NewFrame(req[3:])
	if mode, vn, leap := frm.Flags(); mode != ntp.ModeClient || vn != ntp.Version4 || leap != ntp.LeapNoWarning {
		t.Errorf("flags = mode %d version %d leap %d", mode, vn, leap)
	}
	if got := binary.BigEndian.Uint64(req[3+40:]); got != nonce {
		t.Errorf("transmit timestamp = %#x, want nonce %#x", got, nonce)
	}
	for i, c := range req[3 : 3+40] {
		if c != 0 && i != 0 {
			t.Errorf("request byte %d = %#x, want 0", i, c)
		}
	}
}

func TestParseResponse(t *testing.T) {
	// 40 ms on the wire each way, 2 ms inside the server.
	sent := int64(7 * time.Second)
	recv := sent + int64(82*time.Millisecond)
	resp := response(t, nonce, serverTime, serverTime.Add(2*time.Millisecond))
	res, err := ParseResponse(resp, nonce, sent, recv)
	if err != nil {
		t.Fatal(err)
	}
	if res.RTT != 80*time.Millisecond {
		t.Errorf("RTT = %v, want 80ms", res.RTT)
	}
	want := serverTime.Add(42 * time.Millisecond)
	if d := res.Wall.Sub(want); d < -time.Microsecond || d > time.Microsecond {
		t.Errorf("Wall = %v, want %v", res.Wall, want)
	}
	if res.Stratum != 2 || res.RefID != [4]byte{'G', 'P', 'S', 0} {
		t.Errorf("stratum %d refID %q", res.Stratum, res.RefID[:])
	}
}

func TestParseResponseRejects(t *testing.T) {
	sent, recv := int64(0), int64(50*time.Millisecond)
	tx := serverTime.Add(time.Millisecond)
	tests := []struct {
		name  string
		resp  func() []byte
		nonce uint64
		want  error
	}{
		{"short", func() []byte { return response(t, nonce, serverTime, tx)[:Size-1] }, nonce, ErrNotResponse},
		{"other nonce", func() []byte { return response(t, nonce+1, serverTime, tx) }, nonce, ErrNotResponse},
		{"client mode", func() []byte {
			b := response(t, nonce, serverTime, tx)
			b[0] = b[0]&^7 | byte(ntp.ModeClient)
			return b
		}, nonce, ErrNotResponse},
		{"kiss of death", func() []byte {
			b := response(t, nonce, serverTime, tx)
			b[1] = 0
			copy(b[12:16], "RATE")
			return b
		}, nonce, ErrKissOfDeath},
		{"leap alarm", func() []byte {
			b := response(t, nonce, serverTime, tx)
			b[0] |= 3 << 6
			return b
		}, nonce, ErrUnsynchronized},
		{"stratum 16", func() []byte {
			b := response(t, nonce, serverTime, tx)
			b[1] = 16
			return b
		}, nonce, ErrUnsynchronized},
		{"zero transmit time", func() []byte {
			b := response(t, nonce, serverTime, tx)
			clear(b[40:48])
			return b
		}, nonce, ErrUnsynchronized},
		{"server took longer than the round trip", func() []byte {
			return response(t, nonce, serverTime, serverTime.Add(time.Second))
		}, nonce, ErrNotResponse},
		{"transmitted before received", func() []byte {
			return response(t, nonce, serverTime, serverTime.Add(-time.Millisecond))
		}, nonce, ErrNotResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseResponse(tt.resp(), tt.nonce, sent, recv)
			if err != tt.want {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want == ErrKissOfDeath && string(res.RefID[:]) != "RATE" {
				t.Errorf("kiss code = %q, want RATE", res.RefID[:])
			}
		})
	}
}

func TestEras(t *testing.T) {
	tests := []struct {
		sec  uint32
		want time.Time
	}{
		{2208988800, time.Unix(0, 0)},
		{0xffffffff, time.Date(2036, 2, 7, 6, 28, 15, 0, time.UTC)},
		{0, time.Date(2036, 2, 7, 6, 28, 16, 0, time.UTC)},
		{1 << 30, time.Date(2070, 2, 15, 20, 5, 20, 0, time.UTC)},
	}
	for _, tt := range tests {
		got := unixTime(ntp.TimestampFromUint64(uint64(tt.sec) << 32))
		if !got.Equal(tt.want) {
			t.Errorf("unixTime(%#x) = %v, want %v", tt.sec, got, tt.want)
		}
	}
	half := unixTime(ntp.TimestampFromUint64(uint64(2208988800)<<32 | 1<<31))
	if half.Nanosecond() != 500_000_000 {
		t.Errorf("fraction 2^31 = %d ns, want 500ms", half.Nanosecond())
	}
}

func TestExchangeDoesNotAllocate(t *testing.T) {
	buf := make([]byte, 0, Size)
	resp := response(t, nonce, serverTime, serverTime.Add(time.Millisecond))
	other := response(t, nonce+1, serverTime, serverTime.Add(time.Millisecond))
	allocs := testing.AllocsPerRun(100, func() {
		AppendRequest(buf[:0], nonce)
		if _, err := ParseResponse(resp, nonce, 0, int64(20*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		ParseResponse(other, nonce, 0, int64(20*time.Millisecond))
	})
	if allocs != 0 {
		t.Errorf("exchange allocated %v times per run", allocs)
	}
}
