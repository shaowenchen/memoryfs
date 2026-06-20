package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

// ChunkStore reads and writes file chunks via MemoryFS nodes.
type ChunkStore struct {
	mu            sync.RWMutex
	nodes         []string
	meta          meta.Backend
	transport     transport.ChunkTransport
	replicaFactor int
}

// NewChunkStore creates a chunk store with multi-protocol transport.
func NewChunkStore(metaStore meta.Backend, nodes []string, replicaFactor int) *ChunkStore {
	if replicaFactor <= 0 {
		replicaFactor = chunk.DefaultReplicaFactor
	}
	httpTP := transport.NewHTTPTransport()
	grpcTP := transport.NewGRPCTransport()
	rdmaTP := transport.NewRDMATransport(grpcTP)
	return &ChunkStore{
		nodes:         append([]string(nil), nodes...),
		meta:          metaStore,
		transport:     transport.NewMultiTransport(rdmaTP, grpcTP, httpTP),
		replicaFactor: replicaFactor,
	}
}

// RefreshNodes merges cluster-registered nodes with any configured seed URLs.
func (c *ChunkStore) RefreshNodes(ctx context.Context) error {
	registered, err := c.meta.ListNodes(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodes = mergeNodeURLs(c.nodes, registered)
	return err
}

func mergeNodeURLs(existing, discovered []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(discovered))
	out := make([]string, 0, len(existing)+len(discovered))
	add := func(list []string) {
		for _, n := range list {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	add(existing)
	add(discovered)
	return out
}

// Nodes returns current node URLs.
func (c *ChunkStore) Nodes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.nodes))
	copy(out, c.nodes)
	return out
}

func (c *ChunkStore) selectNodes(chunkID string, rf int) ([]string, error) {
	return chunk.SelectNodes(c.Nodes(), chunkID, rf)
}

func (c *ChunkStore) nodesForChunk(chunkID string) []string {
	primary, err := c.selectNodes(chunkID, c.replicaFactor)
	if err != nil {
		return c.Nodes()
	}
	return mergeNodeURLs(primary, c.Nodes())
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
		data, err := c.readChunk(ctx, chunkID)
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
		existing, _ := c.readChunk(ctx, chunkID)

		buf := make([]byte, meta.ChunkSize)
		copy(buf, existing)
		n := copy(buf[chunkOff:], data[pos:])
		payload := buf[:maxLen(chunkOff+n, len(existing))]
		if err := c.writeChunk(ctx, chunkID, payload); err != nil {
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
			data, err := c.readChunk(ctx, chunkID)
			if err == nil {
				trim := int(size % meta.ChunkSize)
				if trim == 0 && size > 0 {
					trim = meta.ChunkSize
				}
				if trim < len(data) {
					_ = c.writeChunk(ctx, chunkID, data[:trim])
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

func (c *ChunkStore) readChunk(ctx context.Context, chunkID string) ([]byte, error) {
	nodes := c.nodesForChunk(chunkID)
	if len(nodes) == 0 {
		return nil, ErrNoNodes
	}
	var last error
	for _, node := range nodes {
		data, err := c.transport.GetChunk(ctx, node, chunkID)
		if err == nil {
			return data, nil
		}
		last = err
	}
	return nil, last
}

func (c *ChunkStore) writeChunk(ctx context.Context, chunkID string, data []byte) error {
	nodes := c.nodesForChunk(chunkID)
	if len(nodes) == 0 {
		return ErrNoNodes
	}
	var last error
	for _, node := range nodes {
		if err := c.transport.PutChunk(ctx, node, chunkID, data); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return last
}

func (c *ChunkStore) deleteChunk(ctx context.Context, chunkID string) error {
	nodes := c.nodesForChunk(chunkID)
	if len(nodes) == 0 {
		return ErrNoNodes
	}
	var last error
	for _, node := range nodes {
		err := c.transport.DeleteChunk(ctx, node, chunkID)
		if err == nil {
			return nil
		}
		last = err
	}
	return last
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

func (c *ChunkStore) selectNode(chunkID string) (string, error) {
	nodes, err := c.selectNodes(chunkID, 1)
	if err != nil {
		return "", err
	}
	return nodes[0], nil
}

// SelectNode exposes primary node for a chunk (testing).
func (c *ChunkStore) SelectNode(chunkID string) (string, error) {
	return c.selectNode(chunkID)
}

// WriteChunkDirect writes to a specific node (testing).
func (c *ChunkStore) WriteChunkDirect(ctx context.Context, node, chunkID string, data []byte) error {
	return c.transport.PutChunk(ctx, node, chunkID, data)
}

// ReadChunkDirect reads from a specific node (testing).
func (c *ChunkStore) ReadChunkDirect(ctx context.Context, node, chunkID string) ([]byte, error) {
	return c.transport.GetChunk(ctx, node, chunkID)
}

// ErrNoNodes indicates no cluster nodes are available.
var ErrNoNodes = fmt.Errorf("no nodes available")
