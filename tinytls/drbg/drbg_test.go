package drbg

import (
	"bytes"
	"testing"
)

func seeded(seed byte) *Reader {
	r := new(Reader)
	r.Seed(bytes.Repeat([]byte{seed}, MinSeed))
	return r
}

func TestRefusesUntilSeeded(t *testing.T) {
	var r Reader
	r.Seed(make([]byte, MinSeed-1))
	if _, err := r.Read(make([]byte, 8)); err != ErrNotSeeded {
		t.Fatalf("got %v, want ErrNotSeeded", err)
	}
	r.Seed([]byte{1})
	if n, err := r.Read(make([]byte, 8)); n != 8 || err != nil {
		t.Fatalf("got %d, %v", n, err)
	}
}

func TestDeterministicPerSeed(t *testing.T) {
	a, b, c := seeded(1), seeded(1), seeded(2)
	outA, outB, outC := make([]byte, 100), make([]byte, 100), make([]byte, 100)
	a.Read(outA)
	b.Read(outB)
	c.Read(outC)
	if !bytes.Equal(outA, outB) {
		t.Fatal("same seed, different output")
	}
	if bytes.Equal(outA, outC) {
		t.Fatal("different seeds, same output")
	}
}

func TestOutputDoesNotRepeat(t *testing.T) {
	r := seeded(1)
	seen := map[string]bool{}
	buf := make([]byte, 32)
	for i := range 1000 {
		r.Read(buf)
		if seen[string(buf)] {
			t.Fatalf("block %d repeats", i)
		}
		seen[string(buf)] = true
	}
	// Blocks within one Read differ too.
	long := make([]byte, 96)
	r.Read(long)
	if bytes.Equal(long[:32], long[32:64]) || bytes.Equal(long[32:64], long[64:]) {
		t.Fatal("blocks within a Read repeat")
	}
}

func TestSeedChangesOutput(t *testing.T) {
	a, b := seeded(1), seeded(1)
	b.Seed([]byte{0})
	outA, outB := make([]byte, 32), make([]byte, 32)
	a.Read(outA)
	b.Read(outB)
	if bytes.Equal(outA, outB) {
		t.Fatal("reseed did not change the output")
	}
}

func TestReadRekeys(t *testing.T) {
	r := seeded(1)
	key := r.key
	r.Read(make([]byte, 1))
	if r.key == key {
		t.Fatal("Read kept the key")
	}
	if r.block != [32]byte{} {
		t.Fatal("output block left in the state")
	}
}

func TestDoesNotAllocate(t *testing.T) {
	r := seeded(1)
	buf := make([]byte, 96)
	seed := make([]byte, 24)
	if allocs := testing.AllocsPerRun(10, func() {
		r.Seed(seed)
		r.Read(buf)
	}); allocs != 0 {
		t.Fatalf("%v allocations", allocs)
	}
}
