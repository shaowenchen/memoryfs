package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/meta"
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
		nodes:         []string{"http://n1:8080/memoryfs", "http://n2:8080/memoryfs"},
		uriPrefix:     "/memoryfs",
		replicaFactor: 2,
		replicaLookup: &fakeReplicaLookup{
			replicas: map[string][]string{
				"1_0": {"http://n2:8080", "http://n3:8080"},
			},
		},
	}
	order := c.nodesForChunk(context.Background(), "1_0")
	if len(order) != 2 || order[0] != "http://n2:8080/memoryfs" || order[1] != "http://n3:8080/memoryfs" {
		t.Fatalf("expected registry replicas with prefix, got %v", order)
	}
}

func TestNodesForChunkFallsBackToHashSelect(t *testing.T) {
	c := &ChunkStore{
		nodes:         []string{"http://n1:8080/memoryfs", "http://n2:8080/memoryfs"},
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
		seeds: []string{"http://seed:8080/memoryfs"},
		nodes: []string{"http://seed:8080/memoryfs"},
		meta: &fakeMeta{
			nodes: []string{"http://n1:8080/memoryfs", "http://n2:8080/memoryfs"},
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
	got := mergeNodeURLs([]string{"http://a:8080"}, []string{"http://b:8080", "http://a:8080"})
	if len(got) != 2 || got[0] != "http://a:8080" || got[1] != "http://b:8080" {
		t.Fatalf("unexpected merge: %v", got)
	}
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
