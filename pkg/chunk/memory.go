package chunk

import (
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

// Deprecated: use NewMemoryStore.
func NewLocalStore() Store { return NewMemoryStore() }
