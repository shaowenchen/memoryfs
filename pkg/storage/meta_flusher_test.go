package storage

import (
	"context"
	"sync"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

type recordingTransport struct {
	mu   sync.Mutex
	puts []putCall
}

type putCall struct {
	node string
	id   string
	n    int
}

func (t *recordingTransport) Kind() transport.Kind { return transport.KindHTTP }

func (t *recordingTransport) GetChunk(context.Context, string, string) ([]byte, error) {
	return nil, ErrNoNodes
}

func (t *recordingTransport) PutChunk(_ context.Context, node, chunkID string, data []byte) error {
	t.mu.Lock()
	t.puts = append(t.puts, putCall{node: node, id: chunkID, n: len(data)})
	t.mu.Unlock()
	return nil
}

func (t *recordingTransport) PutChunkReplica(ctx context.Context, node, chunkID string, data []byte) error {
	return t.PutChunk(ctx, node, chunkID, data)
}

func (t *recordingTransport) DeleteChunk(context.Context, string, string) error { return nil }

func (t *recordingTransport) putCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.puts)
}

type recordingWriteAt struct {
	mu     sync.Mutex
	writes []writeAtCall
}

type writeAtCall struct {
	ino    uint64
	offset int64
	n      int
}

func (r *recordingWriteAt) WriteAt(_ context.Context, ino uint64, offset int64, data []byte) error {
	r.mu.Lock()
	r.writes = append(r.writes, writeAtCall{ino: ino, offset: offset, n: len(data)})
	r.mu.Unlock()
	return nil
}

func (r *recordingWriteAt) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.writes)
}

func TestWriteThroughOnEveryFUSEWrite(t *testing.T) {
	wa := &recordingWriteAt{}
	c := newChunkStore(nil, nil, 1, &recordingTransport{}, "")
	c.flusher = wa
	attr := &meta.Attr{Ino: 9, Size: 0}

	if err := c.Write(context.Background(), attr, []byte("ab"), 0); err != nil {
		t.Fatal(err)
	}
	if wa.count() != 1 {
		t.Fatalf("expected immediate leader write, got %d", wa.count())
	}
	if err := c.Write(context.Background(), attr, []byte("cd"), 2); err != nil {
		t.Fatal(err)
	}
	if wa.count() != 2 {
		t.Fatalf("expected second immediate write, got %d", wa.count())
	}
}

func TestSmallWriteBufferedUntilLeaderFlush(t *testing.T) {
	tp := &recordingTransport{}
	c := newChunkStore(nil, []string{"http://n1:19800"}, 1, tp, "")
	attr := &meta.Attr{Ino: 9, Size: 0}

	if err := c.Write(context.Background(), attr, []byte("hi"), 0); err != nil {
		t.Fatal(err)
	}
	if tp.putCount() != 0 {
		t.Fatalf("expected no direct chunk PUT without flusher, got %d", tp.putCount())
	}
	if err := c.FlushFile(context.Background(), attr.Ino); err != nil {
		t.Fatal(err)
	}
	if tp.putCount() != 1 {
		t.Fatalf("expected buffered flush via transport, got %d", tp.putCount())
	}
}

func TestFullBlockAutoFlush(t *testing.T) {
	tp := &recordingTransport{}
	c := newChunkStore(nil, []string{"http://n1:19800"}, 1, tp, "")
	attr := &meta.Attr{Ino: 10, Size: 0}
	data := make([]byte, meta.BlockSize)

	if err := c.Write(context.Background(), attr, data, 0); err != nil {
		t.Fatal(err)
	}
	if tp.putCount() != 1 {
		t.Fatalf("expected auto flush for full block, got %d puts", tp.putCount())
	}
}
