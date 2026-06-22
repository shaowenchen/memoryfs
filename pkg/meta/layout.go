package meta

import "fmt"

// Storage layout follows a JuiceFS-style hierarchy:
//   - Chunk (64 MiB): logical file segment indexed in inode metadata (Attr.Chunks)
//   - Block (4 MiB max): physical replica unit; node stores BlockID -> exact-size buffer
const (
	BlockSize = 4 << 20  // 4 MiB — inter-node replica sync unit
	ChunkSize = 64 << 20 // 64 MiB — logical chunk for offset lookup
)

// BlocksPerChunk is the number of blocks in one logical chunk.
const BlocksPerChunk = ChunkSize / BlockSize

// ChunkID returns the logical chunk identifier stored in inode metadata.
func ChunkID(ino uint64, chunkIdx int) string {
	return fmt.Sprintf("%d_%d", ino, chunkIdx)
}

// BlockID returns the physical block identifier used for chunk I/O and replication.
func BlockID(ino uint64, chunkIdx, blockIdx int) string {
	return fmt.Sprintf("%d_%d_%d", ino, chunkIdx, blockIdx)
}

// LegacyChunkID returns the pre-block-layout chunk id (one 4 MiB slice per id).
func LegacyChunkID(ino uint64, legacyIdx int) string {
	return fmt.Sprintf("%d_%d", ino, legacyIdx)
}

// LocateBlock maps a file offset to logical chunk, block, and in-block offset.
func LocateBlock(offset int64) (chunkIdx, blockIdx, blockOff int) {
	if offset < 0 {
		offset = 0
	}
	chunkIdx = int(offset / ChunkSize)
	chunkOff := int(offset % ChunkSize)
	blockIdx = chunkOff / BlockSize
	blockOff = chunkOff % BlockSize
	return chunkIdx, blockIdx, blockOff
}

// LegacyBlockIndex converts logical chunk/block indices to the legacy global slice index.
func LegacyBlockIndex(chunkIdx, blockIdx int) int {
	return chunkIdx*BlocksPerChunk + blockIdx
}
