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

func TestSmallWriteBufferedUntilFlush(t *testing.T) {
	tp := &recordingTransport{}
	c := newChunkStore(nil, []string{"http://n1:19800"}, 1, tp, "")
	attr := &meta.Attr{Ino: 9, Size: 0}

	if err := c.Write(context.Background(), attr, []byte("hi"), 0); err != nil {
		t.Fatal(err)
	}
	if tp.putCount() != 0 {
		t.Fatalf("expected no replica PUT for small write, got %d", tp.putCount())
	}
	if err := c.FlushFile(context.Background(), attr.Ino); err != nil {
		t.Fatal(err)
	}
	if tp.putCount() != 1 {
		t.Fatalf("expected 1 PUT after flush, got %d", tp.putCount())
	}
	if tp.puts[0].id != "9_0_0" || tp.puts[0].n != 2 {
		t.Fatalf("unexpected PUT: %+v", tp.puts[0])
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
	if tp.puts[0].id != "10_0_0" || tp.puts[0].n != meta.BlockSize {
		t.Fatalf("unexpected PUT: %+v", tp.puts[0])
	}
}

func TestLegacyBlockRead(t *testing.T) {
	tp := &legacyReadTransport{data: map[string][]byte{"7_1": []byte("legacy-bytes")}}
	c := newChunkStore(nil, []string{"http://n1:19800"}, 1, tp, "")
	attr := &meta.Attr{Ino: 7, Size: uint64(8 << 20)}

	buf := make([]byte, 12)
	n, err := c.Read(context.Background(), attr, buf, int64(meta.BlockSize))
	if err != nil {
		t.Fatal(err)
	}
	if n != 12 || string(buf[:n]) != "legacy-bytes" {
		t.Fatalf("read legacy block: n=%d buf=%q", n, buf[:n])
	}
}

type legacyReadTransport struct {
	data map[string][]byte
}

func (t *legacyReadTransport) Kind() transport.Kind { return transport.KindHTTP }

func (t *legacyReadTransport) GetChunk(_ context.Context, _, chunkID string) ([]byte, error) {
	if d, ok := t.data[chunkID]; ok {
		return d, nil
	}
	return nil, ErrNoNodes
}

func (t *legacyReadTransport) PutChunk(context.Context, string, string, []byte) error { return nil }
func (t *legacyReadTransport) PutChunkReplica(context.Context, string, string, []byte) error {
	return nil
}
func (t *legacyReadTransport) DeleteChunk(context.Context, string, string) error { return nil }
