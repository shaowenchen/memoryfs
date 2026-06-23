package storage

import (
	"context"
	"sync"
	"time"

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
	// lastAutoFlush gates periodic background-like flushing on write path.
	lastAutoFlush time.Time
}

const (
	// smallWriteImmediateFlushBytes flushes tiny writes quickly so small files
	// become visible/durable in node memory without waiting for fsync/release.
	smallWriteImmediateFlushBytes = 64 << 10 // 64 KiB
	// autoFlushInterval bounds tail latency for partial blocks under sustained
	// small writes while still allowing short batching windows.
	autoFlushInterval = 10 * time.Millisecond
)

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
	var maxSize uint64
	for key, valid := range w.valid {
		if uint64(key.chunkIdx*meta.ChunkSize+valid) > maxSize {
			maxSize = uint64(key.chunkIdx*meta.ChunkSize + valid)
		}
	}
	for key := range w.dirty {
		if err := w.flushLocked(ctx, key, maxSize); err != nil {
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
	oldSize := attr.Size
	end := offset + int64(len(data))
	if end > int64(attr.Size) {
		attr.Size = uint64(end)
	}

	pos := 0
	for pos < len(data) {
		absOff := offset + int64(pos)
		chunkIdx, blockIdx, blockOff := meta.LocateBlock(absOff)
		blockStart := int64(chunkIdx)*meta.ChunkSize + int64(blockIdx)*meta.BlockSize
		remainingInBlock := meta.BlockSize - blockOff
		if remainingInBlock <= 0 {
			remainingInBlock = meta.BlockSize
		}
		writeN := len(data) - pos
		if writeN > remainingInBlock {
			writeN = remainingInBlock
		}
		key := blockKey{chunkIdx: chunkIdx, blockIdx: blockIdx}

		// Fast path: a full 4 MiB aligned block can be sent directly without
		// staging/copying into mount-side block buffers.
		if blockOff == 0 && writeN == meta.BlockSize {
			blockID := meta.BlockID(w.ino, chunkIdx, blockIdx)
			w.mu.Lock()
			delete(w.dirty, key)
			delete(w.valid, key)
			w.mu.Unlock()
			ensureChunk(attr, chunkIdx)
			if err := w.store.writeChunk(ctx, blockID, data[pos:pos+writeN]); err != nil {
				return err
			}
			pos += writeN
			continue
		}

		w.mu.Lock()
		buf := w.dirty[key]
		if buf == nil {
			needRead := blockOff > 0 && oldSize > uint64(blockStart)
			if needRead {
				buf = make([]byte, meta.BlockSize)
				if existing := w.store.readBlock(ctx, attr, chunkIdx, blockIdx); len(existing) > 0 {
					copy(buf, existing)
				}
			}
		}
		need := blockOff + writeN
		if cap(buf) < need {
			grow := make([]byte, need, meta.BlockSize)
			copy(grow, buf)
			buf = grow
		}
		if len(buf) < need {
			buf = buf[:need]
		}
		n := copy(buf[blockOff:], data[pos:pos+writeN])
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
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	needPeriodic := !w.lastAutoFlush.IsZero() && now.Sub(w.lastAutoFlush) >= autoFlushInterval
	needSmallFast := len(data) <= smallWriteImmediateFlushBytes
	if needPeriodic || needSmallFast {
		for key := range w.dirty {
			if err := w.flushLocked(ctx, key, attr.Size); err != nil {
				return err
			}
		}
		w.lastAutoFlush = now
	} else if w.lastAutoFlush.IsZero() {
		w.lastAutoFlush = now
	}
	return nil
}

func (w *blockWriter) flushBlock(ctx context.Context, attr *meta.Attr, chunkIdx, blockIdx int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked(ctx, blockKey{chunkIdx: chunkIdx, blockIdx: blockIdx}, attr.Size)
}

func (w *blockWriter) flushLocked(ctx context.Context, key blockKey, fileSize uint64) error {
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
	payload := buf[:valid]
	blockID := meta.BlockID(w.ino, key.chunkIdx, key.blockIdx)
	if err := w.store.writeChunk(ctx, blockID, payload); err != nil {
		return err
	}
	delete(w.dirty, key)
	delete(w.valid, key)
	return nil
}
