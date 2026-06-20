package storage

import "testing"

func TestNodesForChunkPrefersPrimaryThenAll(t *testing.T) {
	c := &ChunkStore{
		nodes:         []string{"http://n1:8080", "http://n2:8080", "http://n3:8080"},
		replicaFactor: 2,
	}
	order := c.nodesForChunk("1_0")
	if len(order) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(order))
	}
	if order[0] == order[1] || order[1] == order[2] {
		t.Fatalf("duplicate nodes in order: %v", order)
	}
	primary, _ := c.selectNodes("1_0", 2)
	if order[0] != primary[0] || order[1] != primary[1] {
		t.Fatalf("primary replicas should come first: order=%v primary=%v", order, primary)
	}
}

func TestMergeNodeURLs(t *testing.T) {
	got := mergeNodeURLs([]string{"http://a:8080"}, []string{"http://b:8080", "http://a:8080"})
	if len(got) != 2 || got[0] != "http://a:8080" || got[1] != "http://b:8080" {
		t.Fatalf("unexpected merge: %v", got)
	}
}
