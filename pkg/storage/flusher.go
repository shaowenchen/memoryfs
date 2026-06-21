package storage

import (
	"context"
)

// BlockFlusher sends each FUSE write straight to the cluster leader (no mount cache).
type BlockFlusher interface {
	WriteAt(ctx context.Context, ino uint64, offset int64, data []byte) error
}

// BlockFlusherFunc adapts a function to BlockFlusher.
type BlockFlusherFunc func(ctx context.Context, ino uint64, offset int64, data []byte) error

// WriteAt implements BlockFlusher.
func (f BlockFlusherFunc) WriteAt(ctx context.Context, ino uint64, offset int64, data []byte) error {
	return f(ctx, ino, offset, data)
}
