package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/mountlog"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

// ReplicaLookup resolves chunk replica node URLs from cluster metadata.
type ReplicaLookup interface {
	ChunkReplicas(ctx context.Context, chunkID string) ([]string, error)
}

// ChunkStore reads and writes file chunks via MemoryFS nodes.
type ChunkStore struct {
	mu            sync.RWMutex
	seeds         []string
	nodes         []string
	uriPrefix     string
	meta          meta.Backend
	replicaLookup ReplicaLookup
	transport     transport.ChunkTransport
	replicaFactor int
	writers       sync.Map // ino -> *blockWriter
	flusher       BlockFlusher
}

// NewChunkStore creates a chunk store with multi-protocol transport.
func NewChunkStore(metaStore meta.Backend, seeds []string, replicaFactor int) *ChunkStore {
	httpTP := transport.NewHTTPTransport()
	grpcTP := transport.NewGRPCTransport()
	rdmaTP := transport.NewRDMATransport(grpcTP)
	return newChunkStore(metaStore, seeds, replicaFactor, transport.NewMultiTransport(rdmaTP, grpcTP, httpTP), "")
}

// NewHTTPChunkStore creates a chunk store that uses HTTP chunk endpoints only.
func NewHTTPChunkStore(metaStore meta.Backend, seeds []string, replicaFactor int, uriPrefix string) *ChunkStore {
	c := newChunkStore(metaStore, seeds, replicaFactor, transport.NewHTTPTransport(), uriPrefix)
	if rl, ok := metaStore.(ReplicaLookup); ok {
		c.replicaLookup = rl
	}
	return c
}

// NewMountedChunkStore creates a FUSE chunk store: writes go to the leader once
// per block; reads still use replica nodes directly.
func NewMountedChunkStore(metaStore meta.Backend, flusher BlockFlusher, seeds []string, replicaFactor int, uriPrefix string) *ChunkStore {
	c := NewHTTPChunkStore(metaStore, seeds, replicaFactor, uriPrefix)
	// c.flusher = flusher // removed to enable mount-side chunk cache
	return c
}

func newChunkStore(metaStore meta.Backend, seeds []string, replicaFactor int, tp transport.ChunkTransport, uriPrefix string) *ChunkStore {
	if replicaFactor <= 0 {
		replicaFactor = chunk.DefaultReplicaFactor
	}
	seeds = append([]string(nil), seeds...)
	return &ChunkStore{
		seeds:         seeds,
		nodes:         append([]string(nil), seeds...),
		uriPrefix:     strings.TrimSuffix(strings.TrimSpace(uriPrefix), "/"),
		meta:          metaStore,
		transport:     tp,
		replicaFactor: replicaFactor,
	}
}

// RefreshNodes discovers cluster nodes via metadata and merges with seed URLs.
func (c *ChunkStore) RefreshNodes(ctx context.Context) error {
	discovered, err := c.meta.ListNodes(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.nodes = mergeNodeURLs(c.seeds, discovered)
	c.mu.Unlock()
	mountlog.Infof("chunk store nodes refreshed: %v", c.Nodes())
	return nil
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
			key := nodeKey(n)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, n)
		}
	}
	add(existing)
	add(discovered)
	return out
}

func nodeKey(raw string) string {
	raw = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(raw), "http://"), "https://")
	if i := strings.Index(raw, "/"); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimRight(raw, "/")
}

func applyNodePrefix(nodes []string, prefix string) []string {
	prefix = strings.TrimSuffix(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return append([]string(nil), nodes...)
	}
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		n = strings.TrimRight(strings.TrimSpace(n), "/")
		if n == "" {
			continue
		}
		if !strings.HasSuffix(n, prefix) {
			n = n + prefix
		}
		out = append(out, n)
	}
	return out
}

// Nodes returns current cluster node URLs for chunk I/O.
func (c *ChunkStore) Nodes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.nodes))
	copy(out, c.nodes)
	return out
}

func (c *ChunkStore) nodesForChunk(ctx context.Context, chunkID string) []string {
	cluster := applyNodePrefix(c.Nodes(), c.uriPrefix)

	if c.replicaLookup != nil {
		reps, err := c.replicaLookup.ChunkReplicas(ctx, chunkID)
		if err == nil && len(reps) > 0 {
			return applyNodePrefix(reps, c.uriPrefix)
		}
	}

	if selected, err := chunk.SelectNodes(cluster, chunkID, c.replicaFactor); err == nil {
		return selected
	}
	return cluster
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

	written := 0
	pos := offset
	for written < int(toRead) {
		chunkIdx, blockIdx, blockOff := meta.LocateBlock(pos)
		key := blockKey{chunkIdx: chunkIdx, blockIdx: blockIdx}

		var blockData []byte
		if w, ok := c.writers.Load(attr.Ino); ok {
			buf := make([]byte, meta.BlockSize)
			n := w.(*blockWriter).readBuffered(key, buf)
			if n > 0 {
				blockData = buf[:n]
			}
		}
		if blockData == nil {
			blockData = c.readBlock(ctx, attr, chunkIdx, blockIdx)
		}

		avail := meta.BlockSize - blockOff
		want := int(toRead) - written
		if want > avail {
			want = avail
		}
		for i := 0; i < want; i++ {
			if blockOff+i < len(blockData) {
				dest[written+i] = blockData[blockOff+i]
			}
		}
		written += want
		pos += int64(want)
	}
	return written, nil
}

// Write sends data directly to the leader on every FUSE write (no mount buffer).
func (c *ChunkStore) Write(ctx context.Context, attr *meta.Attr, data []byte, offset int64) error {
	if len(data) == 0 {
		return nil
	}
	if c.flusher != nil {
		return c.flusher.WriteAt(ctx, attr.Ino, offset, data)
	}
	return c.writerFor(attr.Ino).Write(ctx, attr, data, offset)
}

// Truncate resizes a file, removing trailing blocks if needed.
func (c *ChunkStore) Truncate(ctx context.Context, attr *meta.Attr, size uint64) error {
	if err := c.FlushFile(ctx, attr.Ino); err != nil {
		return err
	}
	oldSize := attr.Size
	attr.Size = size
	newChunks := chunkCount(size)
	if newChunks < len(attr.Chunks) {
		for i := newChunks; i < len(attr.Chunks); i++ {
			_ = c.deleteLogicalChunk(ctx, attr.Ino, i)
		}
		attr.Chunks = attr.Chunks[:newChunks]
	}
	if size < oldSize {
		_ = c.deleteBlocksFrom(ctx, attr.Ino, size, oldSize)
	}
	if size > 0 {
		lastChunk := int((size - 1) / meta.ChunkSize)
		ensureChunk(attr, lastChunk)
	}
	return nil
}

// DeleteChunks removes all blocks for an inode.
func (c *ChunkStore) DeleteChunks(ctx context.Context, attr *meta.Attr) {
	_ = c.FlushFile(ctx, attr.Ino)
	c.writers.Delete(attr.Ino)
	maxBlocks := blockCount(attr.Size)
	for global := 0; global < maxBlocks; global++ {
		chunkIdx := global / meta.BlocksPerChunk
		blockIdx := global % meta.BlocksPerChunk
		_ = c.deleteBlock(ctx, attr.Ino, chunkIdx, blockIdx)
	}
}

func (c *ChunkStore) readBlock(ctx context.Context, attr *meta.Attr, chunkIdx, blockIdx int) []byte {
	blockID := meta.BlockID(attr.Ino, chunkIdx, blockIdx)
	if data, err := c.readChunk(ctx, blockID); err == nil {
		return data
	}
	legacyID := meta.LegacyChunkID(attr.Ino, meta.LegacyBlockIndex(chunkIdx, blockIdx))
	if data, err := c.readChunk(ctx, legacyID); err == nil {
		return data
	}
	return nil
}

func (c *ChunkStore) deleteLogicalChunk(ctx context.Context, ino uint64, chunkIdx int) error {
	for blockIdx := 0; blockIdx < meta.BlocksPerChunk; blockIdx++ {
		if err := c.deleteBlock(ctx, ino, chunkIdx, blockIdx); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChunkStore) deleteBlock(ctx context.Context, ino uint64, chunkIdx, blockIdx int) error {
	_ = c.deleteChunk(ctx, meta.BlockID(ino, chunkIdx, blockIdx))
	_ = c.deleteChunk(ctx, meta.LegacyChunkID(ino, meta.LegacyBlockIndex(chunkIdx, blockIdx)))
	return nil
}

func (c *ChunkStore) deleteBlocksFrom(ctx context.Context, ino uint64, newSize, oldSize uint64) error {
	for global := blockCount(newSize); global < blockCount(oldSize); global++ {
		chunkIdx := global / meta.BlocksPerChunk
		blockIdx := global % meta.BlocksPerChunk
		_ = c.deleteBlock(ctx, ino, chunkIdx, blockIdx)
	}
	if newSize == 0 {
		return nil
	}
	chunkIdx, blockIdx, _ := meta.LocateBlock(int64(newSize) - 1)
	blockID := meta.BlockID(ino, chunkIdx, blockIdx)
	data, err := c.readChunk(ctx, blockID)
	if err != nil {
		legacyID := meta.LegacyChunkID(ino, meta.LegacyBlockIndex(chunkIdx, blockIdx))
		data, err = c.readChunk(ctx, legacyID)
	}
	if err == nil {
		trim := int(newSize % meta.BlockSize)
		if trim == 0 {
			trim = meta.BlockSize
		}
		if trim < len(data) {
			_ = c.writeChunk(ctx, blockID, data[:trim])
		}
	}
	return nil
}

func (c *ChunkStore) readChunk(ctx context.Context, chunkID string) ([]byte, error) {
	ioCtx, cancel := DetachIOContext(ctx)
	defer cancel()
	nodes := c.nodesForChunk(ioCtx, chunkID)
	if len(nodes) == 0 {
		return nil, ErrNoNodes
	}
	var last error
	for _, node := range nodes {
		data, err := c.transport.GetChunk(ioCtx, node, chunkID)
		if err == nil {
			mountlog.Debugf("chunk %s GET ok node=%s bytes=%d", chunkID, node, len(data))
			return data, nil
		}
		mountlog.Warnf("chunk %s GET failed node=%s: %v", chunkID, node, err)
		last = err
	}
	return nil, last
}

func (c *ChunkStore) writeChunk(ctx context.Context, chunkID string, data []byte) error {
	ioCtx, cancel := DetachIOContext(ctx)
	defer cancel()
	nodes := c.nodesForChunk(ioCtx, chunkID)
	if len(nodes) == 0 {
		return ErrNoNodes
	}
	var last error
	for _, node := range nodes {
		if err := c.transport.PutChunk(ioCtx, node, chunkID, data); err == nil {
			mountlog.Debugf("chunk %s PUT ok node=%s bytes=%d", chunkID, node, len(data))
			return nil
		} else {
			mountlog.Warnf("chunk %s PUT failed node=%s: %v", chunkID, node, err)
			last = err
		}
	}
	mountlog.Errorf("chunk %s PUT all nodes failed: %v", chunkID, last)
	return last
}

func (c *ChunkStore) deleteChunk(ctx context.Context, chunkID string) error {
	ioCtx, cancel := DetachIOContext(ctx)
	defer cancel()
	nodes := c.nodesForChunk(ioCtx, chunkID)
	if len(nodes) == 0 {
		return ErrNoNodes
	}
	var last error
	for _, node := range nodes {
		err := c.transport.DeleteChunk(ioCtx, node, chunkID)
		if err == nil {
			return nil
		}
		last = err
	}
	return last
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

func blockCount(size uint64) int {
	if size == 0 {
		return 0
	}
	return int((size-1)/meta.BlockSize) + 1
}

func (c *ChunkStore) selectNode(ctx context.Context, chunkID string) (string, error) {
	nodes := c.nodesForChunk(ctx, chunkID)
	if len(nodes) == 0 {
		return "", ErrNoNodes
	}
	return nodes[0], nil
}

// SelectNode exposes primary node for a chunk (testing).
func (c *ChunkStore) SelectNode(chunkID string) (string, error) {
	return c.selectNode(context.Background(), chunkID)
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
