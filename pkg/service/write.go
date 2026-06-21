package service

import (
	"context"

	"github.com/shaowenchen/memoryfs/pkg/meta"
)

// WriteBlock stores one file block on the leader (legacy block-index API).
func (s *Service) WriteBlock(ctx context.Context, ino uint64, chunkIdx, blockIdx int, data []byte, fileSize uint64) (*meta.Attr, error) {
	offset := int64(chunkIdx)*meta.ChunkSize + int64(blockIdx)*meta.BlockSize
	attr, err := s.WriteAt(ctx, ino, offset, data)
	if err != nil {
		return nil, err
	}
	if fileSize > attr.Size {
		attr.Size = fileSize
		if err := s.SetAttr(ctx, attr); err != nil {
			return nil, err
		}
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
