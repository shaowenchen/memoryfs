package storage

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/meta"
)

// ChunkStore reads and writes file chunks via MemoryFS workers.
type ChunkStore struct {
	mu      sync.RWMutex
	workers []string
	client  *http.Client
	meta    *meta.Store
}

// NewChunkStore creates a chunk store with optional static worker list.
func NewChunkStore(metaStore *meta.Store, workers []string) *ChunkStore {
	return &ChunkStore{
		workers: workers,
		client:  &http.Client{Timeout: 30 * time.Second},
		meta:    metaStore,
	}
}

// RefreshWorkers reloads worker list from Redis registry.
func (c *ChunkStore) RefreshWorkers(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	registered, err := c.meta.ListWorkers(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(registered) > 0 {
		c.workers = registered
	}
	return nil
}

func (c *ChunkStore) workerList() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.workers))
	copy(out, c.workers)
	return out
}

// Workers returns the current worker URL list.
func (c *ChunkStore) Workers() []string {
	return c.workerList()
}

func (c *ChunkStore) selectWorker(chunkID string) (string, error) {
	workers := c.workerList()
	if len(workers) == 0 {
		return "", fmt.Errorf("no workers available")
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(chunkID))
	return workers[h.Sum32()%uint32(len(workers))], nil
}

// Read reads up to len(dest) bytes at offset from a file identified by attr.
func (c *ChunkStore) Read(ctx context.Context, attr *meta.Attr, dest []byte, offset int64) (int, error) {
	if offset >= int64(attr.Size) {
		return 0, nil
	}
	remaining := int64(attr.Size) - offset
	toRead := int64(len(dest))
	if toRead > remaining {
		toRead = remaining
	}

	chunkIdx := int(offset / meta.ChunkSize)
	chunkOff := int(offset % meta.ChunkSize)
	written := 0

	for written < int(toRead) {
		chunkID := chunkIDFor(attr, chunkIdx)
		data, err := c.getChunk(ctx, chunkID)
		if err != nil {
			return written, err
		}
		n := copy(dest[written:], data[chunkOff:])
		written += n
		chunkIdx++
		chunkOff = 0
	}
	return written, nil
}

// Write writes data at offset, updating attr in place.
func (c *ChunkStore) Write(ctx context.Context, attr *meta.Attr, data []byte, offset int64) error {
	if len(data) == 0 {
		return nil
	}
	end := offset + int64(len(data))
	if end > int64(attr.Size) {
		attr.Size = uint64(end)
	}

	pos := 0
	for pos < len(data) {
		absOff := offset + int64(pos)
		chunkIdx := int(absOff / meta.ChunkSize)
		chunkOff := int(absOff % meta.ChunkSize)

		chunkID := meta.ChunkID(attr.Ino, chunkIdx)
		existing, _ := c.getChunk(ctx, chunkID)

		chunk := make([]byte, meta.ChunkSize)
		copy(chunk, existing)
		n := copy(chunk[chunkOff:], data[pos:])
		if err := c.putChunk(ctx, chunkID, chunk[:maxLen(chunkOff+n, len(existing))]); err != nil {
			return err
		}
		ensureChunk(attr, chunkIdx)
		pos += n
	}
	return nil
}

// Truncate resizes a file, removing trailing chunks if needed.
func (c *ChunkStore) Truncate(ctx context.Context, attr *meta.Attr, size uint64) error {
	oldSize := attr.Size
	attr.Size = size
	newChunks := chunkCount(size)
	if newChunks < len(attr.Chunks) {
		for i := newChunks; i < len(attr.Chunks); i++ {
			_ = c.deleteChunk(ctx, attr.Chunks[i])
		}
		attr.Chunks = attr.Chunks[:newChunks]
	}
	if size < oldSize && size > 0 {
		lastIdx := newChunks - 1
		if lastIdx >= 0 && lastIdx < len(attr.Chunks) {
			chunkID := attr.Chunks[lastIdx]
			data, err := c.getChunk(ctx, chunkID)
			if err == nil {
				trim := int(size % meta.ChunkSize)
				if trim == 0 && size > 0 {
					trim = meta.ChunkSize
				}
				if trim < len(data) {
					_ = c.putChunk(ctx, chunkID, data[:trim])
				}
			}
		}
	}
	return nil
}

// DeleteChunks removes all chunks for an inode.
func (c *ChunkStore) DeleteChunks(ctx context.Context, attr *meta.Attr) {
	for _, id := range attr.Chunks {
		_ = c.deleteChunk(ctx, id)
	}
}

func chunkIDFor(attr *meta.Attr, idx int) string {
	if idx < len(attr.Chunks) && attr.Chunks[idx] != "" {
		return attr.Chunks[idx]
	}
	return meta.ChunkID(attr.Ino, idx)
}

func ensureChunk(attr *meta.Attr, idx int) {
	id := meta.ChunkID(attr.Ino, idx)
	for len(attr.Chunks) <= idx {
		attr.Chunks = append(attr.Chunks, "")
	}
	attr.Chunks[idx] = id
}

func chunkCount(size uint64) int {
	if size == 0 {
		return 0
	}
	return int((size-1)/meta.ChunkSize) + 1
}

func maxLen(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (c *ChunkStore) getChunk(ctx context.Context, chunkID string) ([]byte, error) {
	worker, err := c.selectWorker(chunkID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, worker+"/chunks/"+chunkID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get chunk %s: %s", chunkID, body)
	}
	return io.ReadAll(resp.Body)
}

func (c *ChunkStore) putChunk(ctx context.Context, chunkID string, data []byte) error {
	worker, err := c.selectWorker(chunkID)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, worker+"/chunks/"+chunkID, bytes.NewReader(data))
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put chunk %s: %s", chunkID, body)
	}
	return nil
}

func (c *ChunkStore) deleteChunk(ctx context.Context, chunkID string) error {
	worker, err := c.selectWorker(chunkID)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, worker+"/chunks/"+chunkID, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
