package meta

import "testing"

func TestLocateBlock(t *testing.T) {
	tests := []struct {
		off              int64
		chunk, block, boff int
	}{
		{0, 0, 0, 0},
		{BlockSize - 1, 0, 0, BlockSize - 1},
		{BlockSize, 0, 1, 0},
		{ChunkSize - 1, 0, BlocksPerChunk - 1, BlockSize - 1},
		{ChunkSize, 1, 0, 0},
		{ChunkSize + BlockSize, 1, 1, 0},
	}
	for _, tc := range tests {
		c, b, o := LocateBlock(tc.off)
		if c != tc.chunk || b != tc.block || o != tc.boff {
			t.Fatalf("LocateBlock(%d) = (%d,%d,%d), want (%d,%d,%d)",
				tc.off, c, b, o, tc.chunk, tc.block, tc.boff)
		}
	}
}

func TestLegacyBlockIndex(t *testing.T) {
	if got := LegacyBlockIndex(1, 3); got != 1*BlocksPerChunk+3 {
		t.Fatalf("unexpected legacy index: %d", got)
	}
}
