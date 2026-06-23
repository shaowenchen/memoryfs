package chunk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
//
// memoryfs is a memory-first filesystem: reads and writes hit the chunk
// store directly, with no intermediate read cache. Two backends are
// supported:
//
//   - "memory" (default): chunks live in RAM. DiskQuotaGB caps total
//     payload bytes per node; with quota the store pre-reserves matching
//     buffer slots so allocation amortises (PreallocMemory).
//   - "disk": chunks persist on disk under Dir. DiskQuotaGB rejects
//     puts beyond the quota. Reads/writes go straight to disk (Direct).
type OpenStoreOptions struct {
	Backend     string
	Dir         string
	DiskQuotaGB int64
}

// OpenStoreWithOptions creates a chunk store. The default backend is "memory".
func OpenStoreWithOptions(opt OpenStoreOptions) (Store, error) {
	switch opt.Backend {
	case "", "memory":
		if opt.DiskQuotaGB > 0 {
			return NewPreallocMemory(opt.DiskQuotaGB << 30)
		}
		return NewMemoryStore(), nil
	case "disk":
		if opt.Dir == "" {
			return nil, fmt.Errorf("disk backend requires chunk directory")
		}
		disk, err := NewDiskStore(opt.Dir)
		if err != nil {
			return nil, err
		}
		if opt.DiskQuotaGB > 0 {
			return NewQuotaDisk(disk, opt.DiskQuotaGB<<30), nil
		}
		return disk, nil
	default:
		return nil, fmt.Errorf("unknown chunk backend %q (expected: memory, disk)", opt.Backend)
	}
}
