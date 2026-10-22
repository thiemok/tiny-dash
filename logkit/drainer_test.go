package logkit

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

type recordingWriter struct {
	writes [][]byte
	err    error
}

func (r *recordingWriter) Write(p []byte) (int, error) {
	r.writes = append(r.writes, bytes.Clone(p))
	if r.err != nil {
		return 0, r.err
	}
	return len(p), nil
}

func (r *recordingWriter) joined() string {
	return string(bytes.Join(r.writes, nil))
}

func runUntilIdle(d *Drainer) {
	for d.Step() > 0 {
	}
}

func TestDrainerWritesInChunks(t *testing.T) {
	src := NewRingWriter(make([]byte, 64))
	dst := &recordingWriter{}
	d := Drainer{Src: src, Dst: dst, Chunk: 4}
	src.Write([]byte("hello world\n"))
	runUntilIdle(&d)
	if dst.joined() != "hello world\n" {
		t.Fatalf("got %q", dst.joined())
	}
	for _, w := range dst.writes {
		if len(w) > 4 {
			t.Fatalf("chunk %q exceeds limit", w)
		}
	}
}

func TestDrainerDiscardsWhileNotReady(t *testing.T) {
	src := NewRingWriter(make([]byte, 64))
	dst := &recordingWriter{}
	ready := false
	d := Drainer{Src: src, Dst: dst, Ready: func() bool { return ready }}
	src.Write([]byte("lost\n"))
	runUntilIdle(&d)
	if len(dst.writes) != 0 || src.Buffered() != 0 {
		t.Fatalf("writes=%q buffered=%d", dst.writes, src.Buffered())
	}
	ready = true
	src.Write([]byte("kept\n"))
	runUntilIdle(&d)
	if got := dst.joined(); got != "logkit: dropped=5\nkept\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDrainerDefersDropMarkerToLineBoundary(t *testing.T) {
	src := NewRingWriter(make([]byte, 16))
	dst := &recordingWriter{}
	d := Drainer{Src: src, Dst: dst, Chunk: 4}
	src.Write([]byte("abcdefgh\n"))
	d.Step()
	src.Write([]byte("does not fit\n"))
	runUntilIdle(&d)
	if got := dst.joined(); got != "abcdefgh\nlogkit: dropped=13\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDrainerSkipsDataOnWriteError(t *testing.T) {
	src := NewRingWriter(make([]byte, 64))
	dst := &recordingWriter{err: errors.New("gone")}
	d := Drainer{Src: src, Dst: dst}
	src.Write([]byte("x\n"))
	runUntilIdle(&d)
	if src.Buffered() != 0 {
		t.Fatalf("buffered = %d after write error", src.Buffered())
	}
}

func TestDrainerKeepsDataWhileNotWritable(t *testing.T) {
	src := NewRingWriter(make([]byte, 64))
	dst := &recordingWriter{}
	writable, polls := false, 0
	d := Drainer{Src: src, Dst: dst, Writable: func() bool { polls++; return writable }}
	runUntilIdle(&d)
	if polls != 0 {
		t.Fatalf("Writable polled %d times with nothing pending", polls)
	}
	src.Write([]byte("kept\n"))
	runUntilIdle(&d)
	if len(dst.writes) != 0 || src.Buffered() != 5 || polls != 1 {
		t.Fatalf("writes=%q buffered=%d polls=%d", dst.writes, src.Buffered(), polls)
	}
	writable = true
	runUntilIdle(&d)
	if got := dst.joined(); got != "kept\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDrainerHandlesZeroLengthWrite(t *testing.T) {
	src := NewRingWriter(make([]byte, 64))
	d := Drainer{Src: src, Dst: zeroWriter{}}
	src.Write([]byte("x\n"))
	if n := d.Step(); n != 0 || src.Buffered() != 2 {
		t.Fatalf("n=%d buffered=%d", n, src.Buffered())
	}
}

func TestDrainerReportsDropsAfterReattachMidLine(t *testing.T) {
	src := NewRingWriter(make([]byte, 64))
	dst := &recordingWriter{}
	ready := true
	d := Drainer{Src: src, Dst: dst, Chunk: 2, Ready: func() bool { return ready }}
	src.Write([]byte("abcd\n"))
	d.Step()
	ready = false
	d.Step()
	ready = true
	src.Write([]byte("new\n"))
	runUntilIdle(&d)
	if got := dst.joined(); got != "ablogkit: dropped=3\nnew\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDrainerFlushWaitsForRun(t *testing.T) {
	src := NewRingWriter(make([]byte, 64))
	dst := &recordingWriter{}
	d := Drainer{Src: src, Dst: dst, Idle: time.Millisecond}
	src.Write([]byte("last words\n"))
	if d.Flush(5 * time.Millisecond) {
		t.Fatal("Flush reported success without a running drainer")
	}
	go d.Run()
	if !d.Flush(time.Second) {
		t.Fatalf("Flush timed out, buffered=%d", src.Buffered())
	}
}

func TestDrainerStepDoesNotAllocate(t *testing.T) {
	src := NewRingWriter(make([]byte, 128))
	d := Drainer{Src: src, Dst: discardWriter{}}
	rec := []byte("level=INFO msg=hello\n")
	allocs := testing.AllocsPerRun(1000, func() {
		src.Write(rec)
		runUntilIdle(&d)
	})
	if allocs != 0 {
		t.Fatalf("allocs per step = %v", allocs)
	}
}

func TestDrainerDropMarkerAndNotReadyDoNotAllocate(t *testing.T) {
	src := NewRingWriter(make([]byte, 16))
	ready := true
	d := Drainer{Src: src, Dst: discardWriter{}, Ready: func() bool { return ready }, Writable: func() bool { return true }}
	oversized := []byte("level=INFO msg=does-not-fit\n")
	allocs := testing.AllocsPerRun(1000, func() {
		src.Write(oversized)
		runUntilIdle(&d)
		ready = false
		src.Write(oversized[:8])
		d.Step()
		ready = true
	})
	if allocs != 0 {
		t.Fatalf("allocs per drop cycle = %v", allocs)
	}
	runUntilIdle(&d)
	if d.reported != src.Dropped() {
		t.Fatalf("reported = %d, dropped = %d", d.reported, src.Dropped())
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }
