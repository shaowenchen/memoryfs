package chunk

import (
	"fmt"
)

// PreallocMemory reserves the full quota in RAM at node startup and stores chunks
// within that capacity. Pod memory is allocated up front per storageGB.
type PreallocMemory struct {
	*QuotaMemory
	reserved []byte
}

// NewPreallocMemory creates a quota-backed memory store and retains a quota-sized
// backing slice so RSS is reserved when the pod starts.
func NewPreallocMemory(quotaBytes int64) (*PreallocMemory, error) {
	if quotaBytes <= 0 {
		return nil, fmt.Errorf("prealloc memory requires quota > 0")
	}
	return &PreallocMemory{
		QuotaMemory: NewQuotaMemory(quotaBytes),
		reserved:    make([]byte, quotaBytes),
	}, nil
}

// ReservedBytes returns bytes reserved at startup (may exceed active chunk usage).
func (p *PreallocMemory) ReservedBytes() int64 {
	return int64(len(p.reserved))
}
