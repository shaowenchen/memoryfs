package chunk

// PreallocMemory enforces a byte quota with exact-size buffers (RSS ∝ payload).
// MemoryStore.Put already reuses the existing buffer for in-place RMW when
// capacity is sufficient, so this gives fast updates without 4 MiB-per-chunk
// over-allocation that an arena slot model would impose.
type PreallocMemory struct {
	*QuotaMemory
}

// NewPreallocMemory creates a quota-backed memory chunk store.
func NewPreallocMemory(quotaBytes int64) (*PreallocMemory, error) {
	return &PreallocMemory{QuotaMemory: NewQuotaMemory(quotaBytes)}, nil
}

// ReservedBytes returns the configured chunk quota (for stats/df).
func (p *PreallocMemory) ReservedBytes() int64 {
	return p.quotaBytes
}

// chunkRef returns chunk bytes without copying (internal RMW).
func (p *PreallocMemory) chunkRef(id string) ([]byte, bool) {
	return p.QuotaMemory.chunkRef(id)
}
