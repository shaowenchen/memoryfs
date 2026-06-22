package chunk

import (
	"fmt"
	"testing"
)

func TestBuildChainTable3Nodes2RF(t *testing.T) {
	nodes := []string{"http://n2:19800", "http://n0:19800", "http://n1:19800"}
	tbl, err := BuildChainTable(nodes, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Chains) != 3 {
		t.Fatalf("expected 3 chains, got %d", len(tbl.Chains))
	}
	headCounts := make(map[string]int)
	tailCounts := make(map[string]int)
	for _, c := range tbl.Chains {
		if len(c.Targets) != 2 {
			t.Fatalf("chain %d: expected 2 targets, got %d", c.ID, len(c.Targets))
		}
		if c.Targets[0].Role != RoleHead {
			t.Fatalf("chain %d: target[0] role = %s, want HEAD", c.ID, c.Targets[0].Role)
		}
		if c.Targets[1].Role != RoleTail {
			t.Fatalf("chain %d: target[1] role = %s, want TAIL", c.ID, c.Targets[1].Role)
		}
		if c.Targets[0].NodeURL == c.Targets[1].NodeURL {
			t.Fatalf("chain %d: HEAD and TAIL on same node", c.ID)
		}
		headCounts[c.Targets[0].NodeURL]++
		tailCounts[c.Targets[1].NodeURL]++
	}
	for node, n := range headCounts {
		if n != 1 {
			t.Errorf("node %s has %d HEADs, want 1 (even distribution)", node, n)
		}
	}
	for node, n := range tailCounts {
		if n != 1 {
			t.Errorf("node %s has %d TAILs, want 1", node, n)
		}
	}
}

func TestSelectChainSpreadsHEADs(t *testing.T) {
	nodes := []string{"http://n0:19800", "http://n1:19800", "http://n2:19800"}
	tbl, err := BuildChainTable(nodes, 2)
	if err != nil {
		t.Fatal(err)
	}
	headCounts := make(map[string]int)
	for ino := 9; ino < 9+1024; ino++ {
		chunkID := fmt.Sprintf("%d_0_0", ino)
		chain, err := tbl.SelectChain(chunkID)
		if err != nil {
			t.Fatal(err)
		}
		headCounts[chain.Head().NodeURL]++
	}
	expected := 1024 / 3
	for node, n := range headCounts {
		t.Logf("HEAD=%s chunks=%d expected≈%d", node, n, expected)
		if n < expected*8/10 || n > expected*12/10 {
			t.Errorf("HEAD %s got %d chunks, expected ~%d (uneven)", node, n, expected)
		}
	}
}

func TestChainNextOfReturnsTail(t *testing.T) {
	nodes := []string{"http://n0", "http://n1", "http://n2"}
	chain, err := ChainFor(nodes, "5_0_0", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(chain.Targets) != 3 {
		t.Fatalf("expected RF=3 targets, got %d", len(chain.Targets))
	}
	head := chain.Head().NodeURL
	mid, ok := chain.NextOf(head)
	if !ok || mid.Role != RoleMiddle {
		t.Fatalf("NextOf HEAD: ok=%v role=%v", ok, mid.Role)
	}
	tail, ok := chain.NextOf(mid.NodeURL)
	if !ok || tail.Role != RoleTail {
		t.Fatalf("NextOf MIDDLE: ok=%v role=%v", ok, tail.Role)
	}
	if _, ok := chain.NextOf(tail.NodeURL); ok {
		t.Fatal("NextOf TAIL should return false")
	}
}
