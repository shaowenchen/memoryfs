package storage

import "testing"

func TestNodesForChunkUsesConfiguredOnly(t *testing.T) {
	c := &ChunkStore{
		nodes:         []string{"http://n1:8080"},
		replicaFactor: 2,
	}
	order := c.nodesForChunk("1_0")
	if len(order) != 1 || order[0] != "http://n1:8080" {
		t.Fatalf("expected configured seed only, got %v", order)
	}
}

func TestMergeNodeURLs(t *testing.T) {
	got := mergeNodeURLs([]string{"http://a:8080"}, []string{"http://b:8080", "http://a:8080"})
	if len(got) != 2 || got[0] != "http://a:8080" || got[1] != "http://b:8080" {
		t.Fatalf("unexpected merge: %v", got)
	}
}
