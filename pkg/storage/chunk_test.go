package storage

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

type fakeReplicaLookup struct {
	replicas map[string][]string
}

func (f *fakeReplicaLookup) ChunkReplicas(_ context.Context, chunkID string) ([]string, error) {
	if reps, ok := f.replicas[chunkID]; ok && len(reps) > 0 {
		return reps, nil
	}
	return nil, errors.New("no replicas")
}

func TestNodesForChunkUsesRegistryReplicas(t *testing.T) {
	c := &ChunkStore{
		nodes:         []string{"http://n1:19800/memoryfs", "http://n2:19800/memoryfs"},
		uriPrefix:     "/memoryfs",
		replicaFactor: 2,
		replicaLookup: &fakeReplicaLookup{
			replicas: map[string][]string{
				"1_0": {"http://n2:19800", "http://n3:19800"},
			},
		},
	}
	order := c.nodesForChunk(context.Background(), "1_0")
	if len(order) != 2 || order[0] != "http://n2:19800/memoryfs" || order[1] != "http://n3:19800/memoryfs" {
		t.Fatalf("expected registry replicas with prefix, got %v", order)
	}
}

func TestNodesForChunkFallsBackToHashSelect(t *testing.T) {
	c := &ChunkStore{
		nodes:         []string{"http://n1:19800/memoryfs", "http://n2:19800/memoryfs"},
		uriPrefix:     "/memoryfs",
		replicaFactor: 2,
		replicaLookup: &fakeReplicaLookup{replicas: map[string][]string{}},
	}
	order := c.nodesForChunk(context.Background(), "99_0")
	if len(order) != 2 {
		t.Fatalf("expected hash-selected pair, got %v", order)
	}
}

func TestRefreshNodesMergesDiscovered(t *testing.T) {
	c := &ChunkStore{
		seeds: []string{"http://seed:19800/memoryfs"},
		nodes: []string{"http://seed:19800/memoryfs"},
		meta: &fakeMeta{
			nodes: []string{"http://n1:19800/memoryfs", "http://n2:19800/memoryfs"},
		},
	}
	if err := c.RefreshNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := c.Nodes()
	if len(got) != 3 {
		t.Fatalf("expected 3 nodes, got %v", got)
	}
}

func TestMergeNodeURLs(t *testing.T) {
	got := mergeNodeURLs([]string{"http://a:19800"}, []string{"http://b:19800", "http://a:19800"})
	if len(got) != 2 || got[0] != "http://a:19800" || got[1] != "http://b:19800" {
		t.Fatalf("unexpected merge: %v", got)
	}
}

func TestWriteAndRead(t *testing.T) {
	ctx := context.Background()
	
	// Create a test attribute
	attr := &meta.Attr{
		Ino:    123,
		Size:   0,
		Chunks: []string{},
	}
	
	// Create ChunkStore with mock transport
	mockTransport := &mockTransport{chunks: make(map[string][]byte)}
	store := &ChunkStore{
		transport:     mockTransport,
		replicaFactor: 1,
		nodes:         []string{"mock://localhost"},
	}
	
	// Test 1: Write 8 MiB data (2 full blocks)
	data := make([]byte, 8*1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	if err := store.Write(ctx, attr, data, 0); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	
	if attr.Size != uint64(len(data)) {
		t.Fatalf("Size mismatch: got %d, want %d", attr.Size, len(data))
	}
	
	// Flush to ensure data is written
	if err := store.FlushFile(ctx, attr.Ino); err != nil {
		t.Fatalf("FlushFile failed: %v", err)
	}
	
	// Test 2: Read back the data
	readBuf := make([]byte, len(data))
	n, err := store.Read(ctx, attr, readBuf, 0)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	
	if n != len(data) {
		t.Fatalf("Read length mismatch: got %d, want %d", n, len(data))
	}
	
	// Test 3: Verify data integrity
	for i := 0; i < len(data); i++ {
		if readBuf[i] != data[i] {
			t.Fatalf("Data mismatch at offset %d: got %d, want %d", i, readBuf[i], data[i])
		}
	}
	
	// Test 4: Partial read
	partialBuf := make([]byte, 1024*1024)
	n, err = store.Read(ctx, attr, partialBuf, 2*1024*1024)
	if err != nil {
		t.Fatalf("Partial read failed: %v", err)
	}
	
	if n != len(partialBuf) {
		t.Fatalf("Partial read length mismatch: got %d, want %d", n, len(partialBuf))
	}
	
	// Verify partial read data
	for i := 0; i < len(partialBuf); i++ {
		offset := 2*1024*1024 + i
		if partialBuf[i] != byte(offset%256) {
			t.Fatalf("Partial data mismatch at offset %d: got %d, want %d", i, partialBuf[i], byte(offset%256))
		}
	}
	
	t.Logf("Write and read test passed: wrote and read %d bytes successfully", len(data))
}

func TestWriteAndReadSmallFile(t *testing.T) {
	ctx := context.Background()
	
	attr := &meta.Attr{
		Ino:    456,
		Size:   0,
		Chunks: []string{},
	}
	
	mockTransport := &mockTransport{chunks: make(map[string][]byte)}
	store := &ChunkStore{
		transport:     mockTransport,
		replicaFactor: 1,
		nodes:         []string{"mock://localhost"},
	}
	
	// Test small file (1 KiB)
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}
	
	if err := store.Write(ctx, attr, data, 0); err != nil {
		t.Fatalf("Write small file failed: %v", err)
	}
	
	if err := store.FlushFile(ctx, attr.Ino); err != nil {
		t.Fatalf("FlushFile failed: %v", err)
	}
	
	readBuf := make([]byte, len(data))
	n, err := store.Read(ctx, attr, readBuf, 0)
	if err != nil {
		t.Fatalf("Read small file failed: %v", err)
	}
	
	if n != len(data) {
		t.Fatalf("Read length mismatch: got %d, want %d", n, len(data))
	}
	
	for i := 0; i < len(data); i++ {
		if readBuf[i] != data[i] {
			t.Fatalf("Data mismatch at offset %d: got %d, want %d", i, readBuf[i], data[i])
		}
	}
	
	t.Logf("Small file test passed: %d bytes", len(data))
}

// mockTransport is a simple in-memory transport for testing
type mockTransport struct {
	chunks map[string][]byte
	mu     sync.Mutex
}

func (m *mockTransport) Kind() transport.Kind {
	return transport.KindHTTP
}

func (m *mockTransport) PutChunk(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	return m.PutChunkWithOptions(ctx, nodeURL, chunkID, data, transport.ChunkWriteOptions{})
}

func (m *mockTransport) PutChunkReplica(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	return m.PutChunkWithOptions(ctx, nodeURL, chunkID, data, transport.ChunkWriteOptions{Replica: true})
}

func (m *mockTransport) PutChunkWithOptions(ctx context.Context, nodeURL, chunkID string, data []byte, opts transport.ChunkWriteOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chunks[chunkID] = append([]byte(nil), data...)
	return nil
}

func (m *mockTransport) GetChunk(ctx context.Context, nodeURL, chunkID string) ([]byte, error) {
	return m.GetChunkWithOptions(ctx, nodeURL, chunkID, transport.ChunkReadOptions{})
}

func (m *mockTransport) GetChunkWithOptions(ctx context.Context, nodeURL, chunkID string, opts transport.ChunkReadOptions) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, ok := m.chunks[chunkID]; ok {
		return append([]byte(nil), data...), nil
	}
	return nil, errors.New("chunk not found")
}

func (m *mockTransport) DeleteChunk(ctx context.Context, nodeURL, chunkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.chunks, chunkID)
	return nil
}

type fakeMeta struct {
	nodes []string
}

func (f *fakeMeta) ListNodes(context.Context) ([]string, error) { return f.nodes, nil }
func (f *fakeMeta) GetAttr(context.Context, uint64) (*meta.Attr, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMeta) Lookup(context.Context, uint64, string) (*meta.Attr, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMeta) Readdir(context.Context, uint64) (map[string]*meta.Attr, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMeta) Mkdir(context.Context, uint64, string, uint32, uint32, uint32) (*meta.Attr, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMeta) Create(context.Context, uint64, string, uint32, uint32, uint32) (*meta.Attr, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMeta) Symlink(context.Context, uint64, string, string, uint32, uint32) (*meta.Attr, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMeta) Unlink(context.Context, uint64, string) (*meta.Attr, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMeta) Rmdir(context.Context, uint64, string) error { return errors.New("not implemented") }
func (f *fakeMeta) Rename(context.Context, uint64, uint64, string, string) error {
	return errors.New("not implemented")
}
func (f *fakeMeta) UpdateAttr(context.Context, *meta.Attr) error { return errors.New("not implemented") }
func (f *fakeMeta) ListInos(context.Context) ([]uint64, error)   { return nil, nil }
func (f *fakeMeta) PurgeInode(context.Context, uint64) error     { return nil }
func (f *fakeMeta) Close() error                                 { return nil }
