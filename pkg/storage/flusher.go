package storage

import (
	"context"
)

// BlockFlusher persists a buffered block through the cluster leader: the node
// service selects replica nodes, stores chunk data, and commits metadata.
type BlockFlusher interface {
	PutBlock(ctx context.Context, ino uint64, chunkIdx, blockIdx int, data []byte, fileSize uint64) error
}

// BlockFlusherFunc adapts a function to BlockFlusher.
type BlockFlusherFunc func(ctx context.Context, ino uint64, chunkIdx, blockIdx int, data []byte, fileSize uint64) error

// PutBlock implements BlockFlusher.
func (f BlockFlusherFunc) PutBlock(ctx context.Context, ino uint64, chunkIdx, blockIdx int, data []byte, fileSize uint64) error {
	return f(ctx, ino, chunkIdx, blockIdx, data, fileSize)
}
