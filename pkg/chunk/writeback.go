package chunk

import (
	"sort"
	"sync"
)

// WriteBackStore keeps writes in memory and periodically flushes them to disk.
type WriteBackStore struct {
	mu    sync.Mutex
	mem   *MemoryStore
	disk  Store
	dirty map[string]struct{}
}

// NewWriteBackStore creates a memory-first store with a disk spill target.
func NewWriteBackStore(disk Store) *WriteBackStore {
	return &WriteBackStore{
		mem:   NewMemoryStore(),
		disk:  disk,
		dirty: make(map[string]struct{}),
	}
}

func (s *WriteBackStore) Get(id string) ([]byte, bool) {
	if data, ok := s.mem.Get(id); ok {
		return data, true
	}
	return s.disk.Get(id)
}

func (s *WriteBackStore) Put(id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mem.Put(id, data); err != nil {
		return err
	}
	s.dirty[id] = struct{}{}
	return nil
}

func (s *WriteBackStore) Delete(id string) error {
	s.mu.Lock()
	delete(s.dirty, id)
	s.mu.Unlock()
	_ = s.mem.Delete(id)
	return s.disk.Delete(id)
}

func (s *WriteBackStore) List() []string {
	seen := make(map[string]struct{})
	for _, id := range s.mem.List() {
		seen[id] = struct{}{}
	}
	for _, id := range s.disk.List() {
		seen[id] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *WriteBackStore) Count() int { return len(s.List()) }

// Flush writes dirty chunks to disk and fsyncs the backing store.
func (s *WriteBackStore) Flush() (int, error) {
	s.mu.Lock()
	written := 0
	for id := range s.dirty {
		data, ok := s.mem.Get(id)
		if !ok {
			delete(s.dirty, id)
			continue
		}
		if err := s.disk.Put(id, data); err != nil {
			s.mu.Unlock()
			return written, err
		}
		delete(s.dirty, id)
		written++
	}
	s.mu.Unlock()

	if f, ok := s.disk.(Flusher); ok {
		if _, err := f.Flush(); err != nil {
			return written, err
		}
	}
	return written, nil
}
