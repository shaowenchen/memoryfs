package service

import (
	"context"
	"fmt"

	"github.com/shaowenchen/memoryfs/pkg/meta"
)

// WriteBlock stores one file block: node service picks replica nodes, persists
// chunk data, updates the Raft-backed registry, then commits inode metadata.
func (s *Service) WriteBlock(ctx context.Context, ino uint64, chunkIdx, blockIdx int, data []byte, fileSize uint64) (*meta.Attr, error) {
	if !s.IsLeader() {
		return nil, fmt.Errorf("not leader")
	}
	attr, err := s.GetAttr(ctx, ino)
	if err != nil {
		return nil, err
	}
	if fileSize > attr.Size {
		attr.Size = fileSize
	}
	ensureLogicalChunk(attr, chunkIdx)

	blockID := meta.BlockID(ino, chunkIdx, blockIdx)
	if _, err := s.PutChunk(ctx, blockID, data); err != nil {
		return nil, err
	}
	if err := s.SetAttr(ctx, attr); err != nil {
		return nil, err
	}
	return attr, nil
}

func ensureLogicalChunk(attr *meta.Attr, chunkIdx int) {
	id := meta.ChunkID(attr.Ino, chunkIdx)
	for len(attr.Chunks) <= chunkIdx {
		attr.Chunks = append(attr.Chunks, "")
	}
	attr.Chunks[chunkIdx] = id
}
