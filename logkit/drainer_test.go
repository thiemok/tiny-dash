package logkit

import (
	"bytes"
	"errors"
	"testing"
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

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
