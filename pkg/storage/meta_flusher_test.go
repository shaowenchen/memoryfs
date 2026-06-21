package storage

import (
	"context"
	"sync"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/meta"
)

type recordingFlusher struct {
	mu    sync.Mutex
	puts  []flushCall
}

type flushCall struct {
	ino              uint64
	chunkIdx, blockIdx int
	n                int
	fileSize         uint64
}

func (f *recordingFlusher) PutBlock(_ context.Context, ino uint64, chunkIdx, blockIdx int, data []byte, fileSize uint64) error {
	f.mu.Lock()
	f.puts = append(f.puts, flushCall{ino: ino, chunkIdx: chunkIdx, blockIdx: blockIdx, n: len(data), fileSize: fileSize})
	f.mu.Unlock()
	return nil
}

func (f *recordingFlusher) putCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.puts)
}

func TestSmallWriteBufferedUntilLeaderFlush(t *testing.T) {
	fl := &recordingFlusher{}
	c := newChunkStore(nil, nil, 1, &recordingTransport{}, "")
	c.flusher = fl
	attr := &meta.Attr{Ino: 9, Size: 0}

	if err := c.Write(context.Background(), attr, []byte("hi"), 0); err != nil {
		t.Fatal(err)
	}
	if fl.putCount() != 0 {
		t.Fatalf("expected no leader flush for small write, got %d", fl.putCount())
	}
	if err := c.FlushFile(context.Background(), attr.Ino); err != nil {
		t.Fatal(err)
	}
	if fl.putCount() != 1 {
		t.Fatalf("expected 1 leader flush, got %d", fl.putCount())
	}
	if fl.puts[0].ino != 9 || fl.puts[0].chunkIdx != 0 || fl.puts[0].blockIdx != 0 || fl.puts[0].n != 2 {
		t.Fatalf("unexpected flush: %+v", fl.puts[0])
	}
}

func TestFullBlockLeaderAutoFlush(t *testing.T) {
	fl := &recordingFlusher{}
	c := newChunkStore(nil, nil, 1, &recordingTransport{}, "")
	c.flusher = fl
	attr := &meta.Attr{Ino: 10, Size: 0}
	data := make([]byte, meta.BlockSize)

	if err := c.Write(context.Background(), attr, data, 0); err != nil {
		t.Fatal(err)
	}
	if fl.putCount() != 1 {
		t.Fatalf("expected auto leader flush, got %d puts", fl.putCount())
	}
	if fl.puts[0].n != meta.BlockSize || fl.puts[0].fileSize != meta.BlockSize {
		t.Fatalf("unexpected flush: %+v", fl.puts[0])
	}
}
