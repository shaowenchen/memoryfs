package chunk

import (
	"fmt"
	"testing"
)

func TestSelectNodesDistributionEven(t *testing.T) {
	nodes := []string{
		"http://10.0.0.1:19800",
		"http://10.0.0.2:19800",
		"http://10.0.0.3:19800",
	}
	rf := 2
	counts := make(map[string]int)
	totalBlocks := 256 // 1GB file = 256 x 4MB blocks

	for chunkIdx := 0; chunkIdx < totalBlocks/16; chunkIdx++ {
		for blockIdx := 0; blockIdx < 16; blockIdx++ {
			chunkID := fmt.Sprintf("9_%d_%d", chunkIdx, blockIdx)
			selected, err := SelectNodes(nodes, chunkID, rf)
			if err != nil {
				t.Fatal(err)
			}
			if len(selected) != rf {
				t.Fatalf("expected %d replicas, got %d", rf, len(selected))
			}
			for _, n := range selected {
				counts[n]++
			}
		}
	}

	// With RF=2 and 3 nodes, each node should hold ~2/3 of all blocks
	expected := totalBlocks * rf / len(nodes) // ~170
	for node, count := range counts {
		ratio := float64(count) / float64(expected)
		t.Logf("node=%s blocks=%d expected≈%d ratio=%.2f", node, count, expected, ratio)
		if ratio < 0.5 || ratio > 1.5 {
			t.Errorf("node %s has %d blocks, expected ~%d (ratio %.2f is too skewed)", node, count, expected, ratio)
		}
	}
}
