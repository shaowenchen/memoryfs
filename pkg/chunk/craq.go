package chunk

import "time"

// ChunkState represents local CRAQ visibility of a chunk replica.
type ChunkState string

const (
	ChunkStatePending   ChunkState = "pending"
	ChunkStateCommitted ChunkState = "committed"
)

// ChunkMeta stores per-chunk CRAQ version state on a node.
type ChunkMeta struct {
	ChunkID    string
	ChainID    uint32
	ChainVer   uint64
	UpdateVer  uint64
	CommitVer  uint64
	State      ChunkState
	UpdatedAt  time.Time
}

// Committed reports whether this replica can serve read-committed traffic.
func (m ChunkMeta) Committed() bool {
	return m.State == ChunkStateCommitted && m.CommitVer >= m.UpdateVer
}

// PublicTargetState models a chain target state similar to 3fs.
type PublicTargetState string

const (
	TargetStateServing PublicTargetState = "serving"
	TargetStateSyncing PublicTargetState = "syncing"
	TargetStateWaiting PublicTargetState = "waiting"
	TargetStateOffline PublicTargetState = "offline"
	TargetStateLastSrv PublicTargetState = "lastsrv"
)
