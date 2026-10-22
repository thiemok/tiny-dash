// Command serialstress reproduces USB-CDC log stalls: one goroutine logs in a
// tight loop while a second goroutine only counts ticks. Each line reports how
// long the previous Write blocked, so a stall shows up as a large blockedMs
// while ticks keeps advancing.
//
// Build with -ldflags="-X main.mode=ring" to route output through
// logkit.RingWriter instead of writing to machine.Serial directly.
package main

import (
	"io"
	"machine"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/thiemok/tiny-dash/logkit"
)

var mode = "direct"

var ticks atomic.Uint32

func main() {
	time.Sleep(2 * time.Second)

	var out io.Writer = machine.Serial
	var ring *logkit.RingWriter
	if mode == "ring" {
		ring = logkit.NewRingWriter(make([]byte, 4096))
		drainer := logkit.Drainer{Src: ring, Dst: machine.Serial, Ready: machine.Serial.DTR}
		go drainer.Run()
		out = ring
	}

	go func() {
		for {
			time.Sleep(10 * time.Millisecond)
			ticks.Add(1)
		}
	}()

	start := time.Now()
	line := make([]byte, 0, 128)
	var seq uint32
	var lastBlocked, maxBlocked time.Duration
	for {
		line = append(line[:0], mode...)
		line = append(line, " seq="...)
		line = strconv.AppendUint(line, uint64(seq), 10)
		line = append(line, " upMs="...)
		line = strconv.AppendInt(line, time.Since(start).Milliseconds(), 10)
		line = append(line, " ticks="...)
		line = strconv.AppendUint(line, uint64(ticks.Load()), 10)
		line = append(line, " blockedMs="...)
		line = strconv.AppendInt(line, lastBlocked.Milliseconds(), 10)
		line = append(line, " maxBlockedMs="...)
		line = strconv.AppendInt(line, maxBlocked.Milliseconds(), 10)
		if ring != nil {
			line = append(line, " dropped="...)
			line = strconv.AppendUint(line, ring.Dropped(), 10)
		}
		line = append(line, " ................................\n"...)

		t := time.Now()
		out.Write(line)
		lastBlocked = time.Since(t)
		maxBlocked = max(maxBlocked, lastBlocked)
		seq++
		time.Sleep(time.Millisecond)
	}
}
