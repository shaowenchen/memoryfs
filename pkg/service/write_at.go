package service

import (
	"context"
	"fmt"

	"github.com/shaowenchen/memoryfs/pkg/meta"
)

// WriteAt writes file data at offset on the leader: merges into 4 MiB blocks,
// stores chunks on selected nodes, replicates to peers, then commits metadata.
func (s *Service) WriteAt(ctx context.Context, ino uint64, offset int64, data []byte) (*meta.Attr, error) {
	if !s.IsLeader() {
		return nil, fmt.Errorf("not leader")
	}
	if len(data) == 0 {
		attr, err := s.GetAttr(ctx, ino)
		if err != nil {
			return nil, err
		}
		return attr, nil
	}
	attr, err := s.GetAttr(ctx, ino)
	if err != nil {
		return nil, err
	}
	end := offset + int64(len(data))
	if end > int64(attr.Size) {
		attr.Size = uint64(end)
	}

	pos := 0
	for pos < len(data) {
		absOff := offset + int64(pos)
		chunkIdx, blockIdx, blockOff := meta.LocateBlock(absOff)
		blockID := meta.BlockID(ino, chunkIdx, blockIdx)

		buf := make([]byte, meta.BlockSize)
		if existing, ok := s.cfg.Chunks.Get(blockID); ok {
			copy(buf, existing)
		}
		n := copy(buf[blockOff:], data[pos:])
		valid := blockOff + n
		if existing, ok := s.cfg.Chunks.Get(blockID); ok && len(existing) > valid {
			valid = len(existing)
		}
		replicatePeers := valid >= meta.BlockSize
		if _, err := s.putChunk(ctx, blockID, buf[:valid], replicatePeers); err != nil {
			return nil, err
		}
		ensureLogicalChunk(attr, chunkIdx)
		pos += n
	}

	if err := s.SetAttr(ctx, attr); err != nil {
		return nil, err
	}
	return attr, nil
}
