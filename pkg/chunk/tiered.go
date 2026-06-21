package chunk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// TieredStore keeps hot chunks in memory and persists all data to disk.
type TieredStore struct {
	mem        *MemoryStore
	persistent Store
	maxBytes   int64
	usedBytes  atomic.Int64
}

// NewTieredStore creates a disk-backed store with an in-memory read cache.
func NewTieredStore(persistent Store, maxMemBytes int64) *TieredStore {
	if maxMemBytes <= 0 {
		maxMemBytes = 512 << 20
	}
	return &TieredStore{mem: NewMemoryStore(), persistent: persistent, maxBytes: maxMemBytes}
}

func (s *TieredStore) Get(id string) ([]byte, bool) {
	if data, ok := s.mem.Get(id); ok {
		return data, true
	}
	data, ok := s.persistent.Get(id)
	if !ok {
		return nil, false
	}
	s.cache(id, data)
	return data, true
}

func (s *TieredStore) Put(id string, data []byte) error {
	if err := s.persistent.Put(id, data); err != nil {
		return err
	}
	s.cache(id, data)
	return nil
}

func (s *TieredStore) Delete(id string) error {
	s.evict(id)
	return s.persistent.Delete(id)
}

func (s *TieredStore) List() []string { return s.persistent.List() }

func (s *TieredStore) Count() int { return s.persistent.Count() }

func (s *TieredStore) DiskUsage() (int64, error) {
	switch d := s.persistent.(type) {
	case *QuotaDisk:
		return d.UsageBytes()
	case *DiskStore:
		return d.UsageBytes()
	default:
		return 0, nil
	}
}

func (s *TieredStore) MemUsage() int64 { return s.usedBytes.Load() }

// Flush fsyncs the persistent disk layer.
func (s *TieredStore) Flush() (int, error) {
	return FlushStore(s.persistent)
}

func (s *TieredStore) cache(id string, data []byte) {
	size := int64(len(data))
	if size > s.maxBytes {
		return
	}
	for s.usedBytes.Load()+size > s.maxBytes {
		s.mem = NewMemoryStore()
		s.usedBytes.Store(0)
		break
	}
	_ = s.mem.Put(id, data)
	s.usedBytes.Add(size)
}

func (s *TieredStore) evict(id string) {
	if data, ok := s.mem.Get(id); ok {
		s.usedBytes.Add(-int64(len(data)))
	}
	_ = s.mem.Delete(id)
}

// QuotaDisk wraps DiskStore with a byte quota.
type QuotaDisk struct {
	*DiskStore
	quotaBytes int64
}

// NewQuotaDisk wraps disk store with optional quota (0 = unlimited).
func NewQuotaDisk(disk *DiskStore, quotaBytes int64) *QuotaDisk {
	return &QuotaDisk{DiskStore: disk, quotaBytes: quotaBytes}
}

func (q *QuotaDisk) Put(id string, data []byte) error {
	if q.quotaBytes > 0 {
		used, err := q.UsageBytes()
		if err != nil {
			return err
		}
		oldSize := int64(0)
		if old, ok := q.Get(id); ok {
			oldSize = int64(len(old))
		}
		if used-oldSize+int64(len(data)) > q.quotaBytes {
			return fmt.Errorf("disk quota exceeded")
		}
	}
	return q.DiskStore.Put(id, data)
}

// Flush fsyncs the underlying disk store.
func (q *QuotaDisk) Flush() (int, error) {
	return q.DiskStore.Flush()
}

// UsageBytes returns total bytes used on disk.
func (d *DiskStore) UsageBytes() (int64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var total int64
	err := filepath.Walk(d.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// GCOrphans removes local chunks not referenced in the registry index.
func GCOrphans(local Store, indexed []string) (int, error) {
	index := make(map[string]struct{}, len(indexed))
	for _, id := range indexed {
		index[id] = struct{}{}
	}
	removed := 0
	for _, id := range local.List() {
		if _, ok := index[id]; ok {
			continue
		}
		if err := local.Delete(id); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// OpenStoreOptions configures chunk store creation.
type OpenStoreOptions struct {
	Backend      string
	Dir          string
	MemCacheMB   int64
	DiskQuotaGB  int64
}

// OpenStoreWithOptions creates a chunk store with tiering and quota.
func OpenStoreWithOptions(opt OpenStoreOptions) (Store, error) {
	switch opt.Backend {
	case "memory":
		if opt.DiskQuotaGB > 0 {
			return NewQuotaMemory(opt.DiskQuotaGB << 30), nil
		}
		return NewMemoryStore(), nil
	case "buffered":
		if opt.Dir == "" {
			return nil, fmt.Errorf("buffered backend requires chunk directory")
		}
		disk, err := NewDiskStore(opt.Dir)
		if err != nil {
			return nil, err
		}
		persistent := Store(disk)
		if opt.DiskQuotaGB > 0 {
			persistent = NewQuotaDisk(disk, opt.DiskQuotaGB<<30)
		}
		return NewWriteBackStore(persistent), nil
	case "disk", "tiered", "":
		if opt.Dir == "" {
			return nil, fmt.Errorf("disk backend requires chunk directory")
		}
		disk, err := NewDiskStore(opt.Dir)
		if err != nil {
			return nil, err
		}
		persistent := Store(disk)
		if opt.DiskQuotaGB > 0 {
			persistent = NewQuotaDisk(disk, opt.DiskQuotaGB<<30)
		}
		if opt.Backend == "tiered" || opt.MemCacheMB > 0 {
			memBytes := opt.MemCacheMB << 20
			if memBytes <= 0 {
				memBytes = 512 << 20
			}
			return NewTieredStore(persistent, memBytes), nil
		}
		return persistent, nil
	default:
		return nil, fmt.Errorf("unknown chunk backend: %s", opt.Backend)
	}
}
