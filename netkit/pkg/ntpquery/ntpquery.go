// Package ntpquery builds SNTPv4 client requests and validates the server's
// response (RFC 4330), using lneto's ntp package for the wire format. It does
// no I/O, so any UDP transport can carry the messages, and it does not
// allocate.
//
// A request carries a nonce, not the client's time, in its transmit
// timestamp: the server echoes it as the origin timestamp, which ties the
// response to the request, and a device that boots at the Unix epoch has no
// meaningful time to send anyway. The caller measures send and receive times
// on its own monotonic clock.
package ntpquery

import (
	"errors"
	"time"

	"github.com/soypat/lneto/ntp"
)

// leapAlarm is the leap indicator of a server whose clock is not
// synchronized; lneto's ntp package has no name for it.
const leapAlarm ntp.LeapIndicator = 3

// Size is the length of a request and the minimum length of a response.
const Size = ntp.SizeHeader

var (
	ErrNotResponse = errors.New("ntpquery: not a response to this request")
	// ErrKissOfDeath is a stratum 0 response; Result.RefID holds the kiss
	// code, e.g. RATE or DENY. The server asks the client to back off.
	ErrKissOfDeath    = errors.New("ntpquery: kiss-of-death response")
	ErrUnsynchronized = errors.New("ntpquery: server is not synchronized")
)

// Result is a validated server response.
type Result struct {
	// Wall is the server's time at the instant the response was received.
	Wall time.Time
	// RTT is the round-trip delay without the server's processing time.
	RTT     time.Duration
	Stratum uint8
	// RefID identifies the server's reference clock, or holds the kiss code
	// of a kiss-of-death response.
	RefID [4]byte
}

// AppendRequest appends a client request carrying nonce to dst.
func AppendRequest(dst []byte, nonce uint64) []byte {
	var zero [Size]byte
	n := len(dst)
	dst = append(dst, zero[:]...)
	frm, _ := ntp.NewFrame(dst[n:])
	frm.SetFlags(ntp.ModeClient, ntp.Version4, ntp.LeapNoWarning)
	frm.SetTransmitTime(ntp.TimestampFromUint64(nonce))
	return dst
}

// ParseResponse validates resp as the server's answer to the request
// carrying nonce, which was sent at the monotonic time sent and answered at
// recv, both in nanoseconds on the same base.
//
// ErrNotResponse means resp answers something else, e.g. an earlier attempt,
// or is malformed, and the caller should keep waiting. ErrKissOfDeath and ErrUnsynchronized
// mean the server cannot be trusted for the time.
func ParseResponse(resp []byte, nonce uint64, sent, recv int64) (Result, error) {
	frm, err := ntp.NewFrame(resp)
	if err != nil {
		return Result{}, ErrNotResponse
	}
	mode, _, leap := frm.Flags()
	if mode != ntp.ModeServer || frm.OriginTime().Uint64() != nonce {
		return Result{}, ErrNotResponse
	}
	res := Result{Stratum: uint8(frm.Stratum()), RefID: *frm.ReferenceID()}
	if res.Stratum == 0 {
		return res, ErrKissOfDeath
	}
	rx, tx := frm.ReceiveTime(), frm.TransmitTime()
	if leap == leapAlarm || res.Stratum >= ntp.MaxStratum || tx.IsZero() || rx.IsZero() {
		return res, ErrUnsynchronized
	}
	processing := tx.Sub(rx)
	elapsed := time.Duration(recv - sent)
	if processing < 0 || elapsed < processing {
		return Result{}, ErrNotResponse
	}
	res.RTT = elapsed - processing
	// The server sent its answer half the round trip before it arrived.
	res.Wall = unixTime(tx).Add(res.RTT / 2)
	return res, nil
}

// eraSplit is the Unix time of NTP era 1's start, 2036-02-07T06:28:16Z.
// RFC 4330 §3: a timestamp with the top bit clear is in era 1, which keeps
// timestamps correct until 2104.
const eraSplit = 1<<32 - 2208988800

func unixTime(ts ntp.Timestamp) time.Time {
	sec := int64(ts.Seconds())
	if sec&(1<<31) != 0 {
		sec -= 2208988800
	} else {
		sec += eraSplit
	}
	nsec := int64(uint64(ts.Fractions()) * uint64(time.Second) >> 32)
	return time.Unix(sec, nsec).UTC()
}
