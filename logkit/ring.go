// Package logkit provides a non-blocking log sink for targets whose output
// device can stall, such as TinyGo's USB-CDC serial, whose Write spins until
// the host reads.
package logkit

import "sync"

// RingWriter is an io.Writer that never blocks: writes are copied into a
// fixed-size ring buffer and dropped whole (and counted) when they do not fit.
// A single consumer drains it, typically via Drainer.
type RingWriter struct {
	mu      sync.Mutex
	buf     []byte
	head    uint64
	tail    uint64
	dropped uint64
}

// NewRingWriter returns a RingWriter backed by buf, which it takes ownership of.
func NewRingWriter(buf []byte) *RingWriter {
	return &RingWriter{buf: buf}
}

// Write copies p into the ring or, if it does not fit entirely, drops it so
// that only whole records are ever emitted. It always reports success.
func (w *RingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	size := uint64(len(w.buf))
	if uint64(len(p)) > size-(w.head-w.tail) {
		w.dropped += uint64(len(p))
		return len(p), nil
	}
	pos := w.head % size
	n := copy(w.buf[pos:], p)
	copy(w.buf, p[n:])
	w.head += uint64(len(p))
	return len(p), nil
}

// Dropped returns the total number of bytes dropped since creation.
func (w *RingWriter) Dropped() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dropped
}

// Buffered returns the number of bytes waiting to be drained.
func (w *RingWriter) Buffered() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return int(w.head - w.tail)
}

// Peek returns up to max buffered bytes as one contiguous slice without
// consuming them. The slice stays valid until the next Discard or Reset; only
// the single consumer may call Peek, Discard and Reset.
func (w *RingWriter) Peek(max int) []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	used := w.head - w.tail
	if used == 0 {
		return nil
	}
	size := uint64(len(w.buf))
	pos := w.tail % size
	end := min(pos+used, size, pos+uint64(max))
	return w.buf[pos:end]
}

// Discard consumes n bytes previously returned by Peek.
func (w *RingWriter) Discard(n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tail += uint64(min(uint64(n), w.head-w.tail))
}

// Reset drops everything buffered, counting it as dropped.
func (w *RingWriter) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dropped += w.head - w.tail
	w.tail = w.head
}
