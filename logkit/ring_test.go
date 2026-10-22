package logkit

import (
	"bytes"
	"strconv"
	"sync"
	"testing"
)

func drainAll(w *RingWriter, max int) []byte {
	var out []byte
	for {
		p := w.Peek(max)
		if len(p) == 0 {
			return out
		}
		out = append(out, p...)
		w.Discard(len(p))
	}
}

func TestRingWriterRoundTripAcrossWrap(t *testing.T) {
	w := NewRingWriter(make([]byte, 16))
	var want []byte
	for i := range 20 {
		rec := []byte("rec" + strconv.Itoa(i) + "\n")
		w.Write(rec)
		want = append(want, rec...)
		got := drainAll(w, 5)
		if !bytes.Equal(got, rec) {
			t.Fatalf("record %d: got %q, want %q", i, got, rec)
		}
	}
	if w.Dropped() != 0 {
		t.Fatalf("dropped = %d, want 0", w.Dropped())
	}
}

func TestRingWriterDropsWholeRecordsWhenFull(t *testing.T) {
	w := NewRingWriter(make([]byte, 10))
	n, err := w.Write([]byte("abcdef"))
	if n != 6 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	n, err = w.Write([]byte("ghijk"))
	if n != 5 || err != nil {
		t.Fatalf("Write on full ring = %d, %v; want success", n, err)
	}
	w.Write([]byte("xyz"))
	if got := w.Dropped(); got != 5 {
		t.Fatalf("dropped = %d, want 5", got)
	}
	if got := string(drainAll(w, 64)); got != "abcdefxyz" {
		t.Fatalf("drained %q", got)
	}
}

func TestRingWriterOversizedRecordDropped(t *testing.T) {
	w := NewRingWriter(make([]byte, 4))
	w.Write([]byte("too long"))
	if w.Buffered() != 0 || w.Dropped() != 8 {
		t.Fatalf("buffered=%d dropped=%d", w.Buffered(), w.Dropped())
	}
}

func TestRingWriterPeekIsContiguous(t *testing.T) {
	w := NewRingWriter(make([]byte, 8))
	w.Write([]byte("123456"))
	w.Discard(len(w.Peek(6)))
	w.Write([]byte("abcde"))
	if p := w.Peek(64); string(p) != "ab" {
		t.Fatalf("first peek = %q, want up to buffer end", p)
	}
	w.Discard(2)
	if p := w.Peek(2); string(p) != "cd" {
		t.Fatalf("limited peek = %q", p)
	}
}

func TestRingWriterResetCountsDropped(t *testing.T) {
	w := NewRingWriter(make([]byte, 8))
	w.Write([]byte("abc"))
	w.Reset()
	if w.Buffered() != 0 || w.Dropped() != 3 {
		t.Fatalf("buffered=%d dropped=%d", w.Buffered(), w.Dropped())
	}
}

func TestRingWriterWriteDoesNotAllocate(t *testing.T) {
	w := NewRingWriter(make([]byte, 64))
	rec := []byte("level=INFO msg=hello\n")
	allocs := testing.AllocsPerRun(1000, func() {
		w.Write(rec)
		w.Discard(len(w.Peek(64)))
	})
	if allocs != 0 {
		t.Fatalf("allocs per write = %v", allocs)
	}
}

func TestRingWriterConcurrentWritersKeepRecordsIntact(t *testing.T) {
	w := NewRingWriter(make([]byte, 256))
	const writers, records = 4, 500
	var wg sync.WaitGroup
	for g := range writers {
		wg.Go(func() {
			rec := []byte("writer-" + strconv.Itoa(g) + "-payload\n")
			for range records {
				w.Write(rec)
			}
		})
	}
	var out []byte
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	for finished := false; !finished; {
		select {
		case <-done:
			finished = true
		default:
		}
		out = append(out, drainAll(w, 7)...)
	}
	lines := bytes.Split(bytes.TrimSuffix(out, []byte("\n")), []byte("\n"))
	for _, l := range lines {
		if !bytes.HasPrefix(l, []byte("writer-")) || !bytes.HasSuffix(l, []byte("-payload")) {
			t.Fatalf("torn record %q", l)
		}
	}
	total := uint64(len(out)) + w.Dropped()
	if want := uint64(writers * records * len("writer-0-payload\n")); total != want {
		t.Fatalf("written+dropped = %d, want %d", total, want)
	}
}
