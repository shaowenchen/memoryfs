package chunk

import (
	"fmt"
)

// PreallocMemory enforces a byte quota for in-memory chunks (storageGB).
// RSS grows with chunk data up to the quota; no separate unused reserve buffer.
type PreallocMemory struct {
	*QuotaMemory
	quotaBytes int64
}

// NewPreallocMemory creates a quota-backed memory chunk store.
func NewPreallocMemory(quotaBytes int64) (*PreallocMemory, error) {
	if quotaBytes <= 0 {
		return nil, fmt.Errorf("prealloc memory requires quota > 0")
	}
	return &PreallocMemory{
		QuotaMemory: NewQuotaMemory(quotaBytes),
		quotaBytes:  quotaBytes,
	}, nil
}

// ReservedBytes returns the configured chunk quota (for stats/df).
func (p *PreallocMemory) ReservedBytes() int64 {
	return p.quotaBytes
}

// chunkRef returns chunk bytes without copying (internal RMW).
func (p *PreallocMemory) chunkRef(id string) ([]byte, bool) {
	return p.MemoryStore.chunkRef(id)
}
