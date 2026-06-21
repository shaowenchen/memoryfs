package storage

import (
	"context"
	"sync"

	"github.com/shaowenchen/memoryfs/pkg/meta"
)

type blockKey struct {
	chunkIdx int
	blockIdx int
}

type blockWriter struct {
	store *ChunkStore
	ino   uint64
	mu    sync.Mutex
	dirty map[blockKey][]byte
	valid map[blockKey]int
}

func (c *ChunkStore) writerFor(ino uint64) *blockWriter {
	if w, ok := c.writers.Load(ino); ok {
		return w.(*blockWriter)
	}
	w := &blockWriter{
		store: c,
		ino:   ino,
		dirty: make(map[blockKey][]byte),
		valid: make(map[blockKey]int),
	}
	actual, _ := c.writers.LoadOrStore(ino, w)
	return actual.(*blockWriter)
}

// FlushFile persists all buffered blocks for an inode.
func (c *ChunkStore) FlushFile(ctx context.Context, ino uint64) error {
	if w, ok := c.writers.Load(ino); ok {
		return w.(*blockWriter).Flush(ctx)
	}
	return nil
}

func (w *blockWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for key := range w.dirty {
		if err := w.flushLocked(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (w *blockWriter) readBuffered(key blockKey, dest []byte) int {
	buf, ok := w.dirty[key]
	if !ok {
		return 0
	}
	valid := w.valid[key]
	if valid > len(buf) {
		valid = len(buf)
	}
	return copy(dest, buf[:valid])
}

func (w *blockWriter) Write(ctx context.Context, attr *meta.Attr, data []byte, offset int64) error {
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
		chunkIdx, blockIdx, blockOff := meta.LocateBlock(absOff)

		w.mu.Lock()
		key := blockKey{chunkIdx: chunkIdx, blockIdx: blockIdx}
		buf := w.dirty[key]
		if buf == nil {
			existing := w.store.readBlock(ctx, attr, chunkIdx, blockIdx)
			buf = make([]byte, meta.BlockSize)
			copy(buf, existing)
		}
		n := copy(buf[blockOff:], data[pos:])
		w.dirty[key] = buf
		prev := w.valid[key]
		if endOff := blockOff + n; endOff > prev {
			w.valid[key] = endOff
		}
		shouldFlush := w.valid[key] >= meta.BlockSize
		w.mu.Unlock()

		ensureChunk(attr, chunkIdx)
		if shouldFlush {
			if err := w.flushBlock(ctx, attr, chunkIdx, blockIdx); err != nil {
				return err
			}
		}
		pos += n
	}
	return nil
}

func (w *blockWriter) flushBlock(ctx context.Context, attr *meta.Attr, chunkIdx, blockIdx int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked(ctx, blockKey{chunkIdx: chunkIdx, blockIdx: blockIdx})
}

func (w *blockWriter) flushLocked(ctx context.Context, key blockKey) error {
	buf, ok := w.dirty[key]
	if !ok {
		return nil
	}
	valid := w.valid[key]
	if valid <= 0 {
		delete(w.dirty, key)
		delete(w.valid, key)
		return nil
	}
	if valid > len(buf) {
		valid = len(buf)
	}
	blockID := meta.BlockID(w.ino, key.chunkIdx, key.blockIdx)
	if err := w.store.writeChunk(ctx, blockID, buf[:valid]); err != nil {
		return err
	}
	delete(w.dirty, key)
	delete(w.valid, key)
	return nil
}
