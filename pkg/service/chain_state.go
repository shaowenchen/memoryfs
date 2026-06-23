package service

import (
	"context"
	"fmt"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/lifecycle"
)

// ChainState exposes local target state on one chain.
type ChainState struct {
	ChainID  uint32                  `json:"chain_id"`
	ChainVer uint64                  `json:"chain_ver"`
	Role     string                  `json:"role"`
	State    chunk.PublicTargetState `json:"state"`
}

// SyncStartResponse returns chunk metadata used by predecessor-driven sync.
type SyncStartResponse struct {
	ChainID  uint32            `json:"chain_id"`
	ChainVer uint64            `json:"chain_ver"`
	Chunks   []chunk.ChunkMeta `json:"chunks"`
}

func (s *Service) recomputeChainStates(ctx context.Context) {
	nodes, err := s.cfg.Meta.ListNodes(ctx)
	if err != nil || len(nodes) == 0 {
		return
	}
	table, err := chunk.BuildChainTable(nodes, s.cfg.ReplicaFactor)
	if err != nil {
		return
	}
	epoch := s.syncClusterEpoch()
	life := s.cfg.Lifecycle.State()

	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for _, c := range table.Chains {
		idx := -1
		for i, t := range c.Targets {
			if t.NodeURL == s.cfg.NodeHTTP {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		state := chunk.TargetStateServing
		if life == lifecycle.StateDraining {
			state = chunk.TargetStateWaiting
		}
		if life == lifecycle.StateDrained {
			state = chunk.TargetStateOffline
		}
		if s.syncingChain[c.ID] {
			state = chunk.TargetStateSyncing
		}
		s.chainStates[c.ID] = state
		if v, ok := s.chainVers[c.ID]; !ok || v < epoch {
			s.chainVers[c.ID] = epoch
		}
	}
}

func (s *Service) chainVersion(chainID uint32) uint64 {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	v := s.chainVers[chainID]
	if v == 0 {
		v = s.syncClusterEpoch()
		s.chainVers[chainID] = v
	}
	return v
}

func (s *Service) markChainSyncing(chainID uint32, on bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if on {
		s.syncingChain[chainID] = true
		s.chainStates[chainID] = chunk.TargetStateSyncing
		return
	}
	delete(s.syncingChain, chainID)
	s.chainStates[chainID] = chunk.TargetStateServing
}

// GetChainState returns local chain status.
func (s *Service) GetChainState(ctx context.Context, chainID uint32) ChainState {
	s.recomputeChainStates(ctx)
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	st := s.chainStates[chainID]
	if st == "" {
		st = chunk.TargetStateOffline
	}
	return ChainState{
		ChainID:  chainID,
		ChainVer: s.chainVers[chainID],
		Role:     "member",
		State:    st,
	}
}

// SyncStart enters syncing state and returns local chunk metadata for chain.
func (s *Service) SyncStart(ctx context.Context, chainID uint32, chainVer uint64) (SyncStartResponse, error) {
	s.markChainSyncing(chainID, true)
	s.stateMu.Lock()
	if chainVer > s.chainVers[chainID] {
		s.chainVers[chainID] = chainVer
	}
	v := s.chainVers[chainID]
	s.stateMu.Unlock()

	nodes, err := s.cfg.Meta.ListNodes(ctx)
	if err != nil {
		return SyncStartResponse{}, err
	}
	localIDs := s.cfg.Chunks.List()
	metas := make([]chunk.ChunkMeta, 0, len(localIDs))
	for _, id := range localIDs {
		ch, err := chunk.ChainFor(nodes, id, s.cfg.ReplicaFactor)
		if err != nil || ch.ID != chainID {
			continue
		}
		meta := s.getMeta(id)
		if meta.ChunkID == "" {
			meta = chunk.ChunkMeta{
				ChunkID:   id,
				ChainID:   chainID,
				ChainVer:  v,
				UpdateVer: 1,
				CommitVer: 1,
				State:     chunk.ChunkStateCommitted,
				UpdatedAt: time.Now(),
			}
		}
		metas = append(metas, meta)
	}
	return SyncStartResponse{ChainID: chainID, ChainVer: v, Chunks: metas}, nil
}

// SyncDone marks local chain synced and ready for serving.
func (s *Service) SyncDone(ctx context.Context, chainID uint32, chainVer uint64) error {
	if chainID == 0 && chainVer == 0 {
		return fmt.Errorf("missing chain id/version")
	}
	s.markChainSyncing(chainID, false)
	s.stateMu.Lock()
	if chainVer > s.chainVers[chainID] {
		s.chainVers[chainID] = chainVer
	}
	s.stateMu.Unlock()
	s.recomputeChainStates(ctx)
	return nil
}
