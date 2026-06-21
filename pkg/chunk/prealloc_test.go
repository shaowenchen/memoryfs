package chunk

import "testing"

func TestPreallocMemoryReservesQuota(t *testing.T) {
	const quota = 32 << 20
	p, err := NewPreallocMemory(quota)
	if err != nil {
		t.Fatal(err)
	}
	if p.ReservedBytes() != quota {
		t.Fatalf("reserved: got %d want %d", p.ReservedBytes(), quota)
	}
	if err := p.Put("1_0_0", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if p.UsageBytes() != 1 {
		t.Fatalf("usage: got %d", p.UsageBytes())
	}
}
