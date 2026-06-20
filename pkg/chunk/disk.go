package chunk

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const chunkIndexKey = "memoryfs:chunkindex"

// DiskStore persists chunks on local host disk.
type DiskStore struct {
	mu  sync.RWMutex
	dir string
}

// NewDiskStore creates a disk-backed chunk store under dir.
func NewDiskStore(dir string) (*DiskStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create chunk dir: %w", err)
	}
	return &DiskStore{dir: dir}, nil
}

func (s *DiskStore) chunkPath(id string) string {
	// shard by first byte pair for large clusters: e.g. "123_0" -> "12/123_0"
	if len(id) >= 2 {
		return filepath.Join(s.dir, id[:2], id)
	}
	return filepath.Join(s.dir, id)
}

func (s *DiskStore) Get(id string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.chunkPath(id))
	if err != nil {
		return nil, false
	}
	return data, true
}

func (s *DiskStore) Put(id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.chunkPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *DiskStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.chunkPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *DiskStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	_ = filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		rel, err := filepath.Rel(s.dir, path)
		if err != nil {
			return nil
		}
		// path like "12/123_0" -> "123_0"
		id := filepath.Base(rel)
		out = append(out, id)
		return nil
	})
	sort.Strings(out)
	return out
}

func (s *DiskStore) Count() int {
	return len(s.List())
}

// Flush fsyncs all chunk files and the storage directory.
func (s *DiskStore) Flush() (int, error) {
	s.mu.RLock()
	dir := s.dir
	s.mu.RUnlock()

	synced := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		_ = f.Close()
		synced++
		return nil
	})
	if err != nil {
		return synced, err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return synced, nil
}

// Dir returns the chunk storage directory.
func (s *DiskStore) Dir() string { return s.dir }

// OpenStore creates the requested chunk backend.
func OpenStore(backend, dir string) (Store, error) {
	switch backend {
	case "memory":
		return NewMemoryStore(), nil
	case "disk", "":
		if dir == "" {
			return nil, fmt.Errorf("disk backend requires chunk directory")
		}
		return NewDiskStore(dir)
	default:
		return nil, fmt.Errorf("unknown chunk backend: %s", backend)
	}
}
