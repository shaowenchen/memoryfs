package chunk

import (
	"fmt"
	"sort"
	"sync"
)

// MemoryStore keeps chunks in process memory (non-persistent).
type MemoryStore struct {
	mu     sync.RWMutex
	chunks map[string][]byte
}

// NewMemoryStore creates an in-memory chunk store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{chunks: make(map[string][]byte)}
}

func (s *MemoryStore) Get(id string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.chunks[id]
	if !ok {
		return nil, false
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, true
}

func (s *MemoryStore) Put(id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	s.chunks[id] = cp
	return nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chunks, id)
	return nil
}

func (s *MemoryStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.chunks))
	for id := range s.chunks {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *MemoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.chunks)
}

// UsageBytes returns total bytes held in memory chunks.
func (s *MemoryStore) UsageBytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var n int64
	for _, data := range s.chunks {
		n += int64(len(data))
	}
	return n
}

// QuotaMemory is an in-memory chunk store with a byte capacity (from storageGB).
// Capacity is enforced on write; no placeholder chunks are allocated at startup.
type QuotaMemory struct {
	*MemoryStore
	quotaBytes int64
}

// NewQuotaMemory creates a memory store limited to quotaBytes (0 = unlimited).
func NewQuotaMemory(quotaBytes int64) *QuotaMemory {
	return &QuotaMemory{MemoryStore: NewMemoryStore(), quotaBytes: quotaBytes}
}

func (q *QuotaMemory) Put(id string, data []byte) error {
	if q.quotaBytes > 0 {
		used := q.UsageBytes()
		oldSize := int64(0)
		if old, ok := q.MemoryStore.Get(id); ok {
			oldSize = int64(len(old))
		}
		if used-oldSize+int64(len(data)) > q.quotaBytes {
			return fmt.Errorf("memory quota exceeded")
		}
	}
	return q.MemoryStore.Put(id, data)
}

// Deprecated: use NewMemoryStore.
func NewLocalStore() Store { return NewMemoryStore() }
