package chunk

import "testing"

func TestQuotaMemoryCapacity(t *testing.T) {
	const chunkSize = 4 << 20
	q := NewQuotaMemory(chunkSize * 2)
	if err := q.Put("a", make([]byte, chunkSize)); err != nil {
		t.Fatal(err)
	}
	if err := q.Put("b", make([]byte, chunkSize)); err != nil {
		t.Fatal(err)
	}
	if err := q.Put("c", []byte("x")); err == nil {
		t.Fatal("expected quota exceeded")
	}
	if got := q.UsageBytes(); got != chunkSize*2 {
		t.Fatalf("usage: got %d want %d", got, chunkSize*2)
	}
}

func TestMemoryStoreUsageBytes(t *testing.T) {
	m := NewMemoryStore()
	_ = m.Put("1_0", []byte("abc"))
	if got := m.UsageBytes(); got != 3 {
		t.Fatalf("usage: %d", got)
	}
}
