package logkit

import (
	"io"
	"strconv"
	"time"
)

const (
	defaultChunk = 64
	defaultIdle  = 10 * time.Millisecond
)

// Drainer moves bytes from a RingWriter to a possibly blocking io.Writer in
// small chunks, so that only the drainer's goroutine stalls when Dst does.
type Drainer struct {
	Src *RingWriter
	Dst io.Writer
	// Ready reports whether anyone is listening on Dst. While it returns false
	// the ring is discarded instead of drained. Nil means always ready.
	Ready func() bool
	// Chunk is the maximum number of bytes per Dst.Write; 0 means 64, one
	// USB full-speed bulk packet.
	Chunk int
	// Idle is how long Run sleeps when there is nothing to do; 0 means 10ms.
	Idle time.Duration

	reported uint64
	midLine  bool
	marker   [48]byte
}

// Step performs one unit of work and returns the number of bytes written to
// Dst. Once new drops have been counted it emits a "logkit: dropped=N" line,
// deferred until the output is at a line boundary.
func (d *Drainer) Step() int {
	if d.Ready != nil && !d.Ready() {
		d.Src.Reset()
		return 0
	}
	if dropped := d.Src.Dropped(); dropped != d.reported && !d.midLine {
		d.reported = dropped
		m := append(d.marker[:0], "logkit: dropped="...)
		m = strconv.AppendUint(m, dropped, 10)
		m = append(m, '\n')
		n, _ := d.Dst.Write(m)
		return n
	}
	chunk := d.Chunk
	if chunk <= 0 {
		chunk = defaultChunk
	}
	p := d.Src.Peek(chunk)
	if len(p) == 0 {
		return 0
	}
	n, err := d.Dst.Write(p)
	if err != nil {
		n = len(p)
	}
	d.midLine = p[n-1] != '\n'
	d.Src.Discard(n)
	return n
}

// Run calls Step forever, sleeping for Idle whenever Step had nothing to do.
func (d *Drainer) Run() {
	idle := d.Idle
	if idle <= 0 {
		idle = defaultIdle
	}
	for {
		if d.Step() == 0 {
			time.Sleep(idle)
		}
	}
}
