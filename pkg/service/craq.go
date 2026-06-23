package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

const (
	stagePrepare = "prepare"
	stageCommit  = "commit"
	stageRemove  = "remove"
)

type ReplicaWrite struct {
	Stage      string
	ChainID    uint32
	ChainVer   uint64
	UpdateVer  uint64
	CommitVer  uint64
	Replicas   []string
	FromClient bool
	Syncing    bool
}

func (s *Service) chunkLock(chunkID string) *sync.Mutex {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if l, ok := s.lockByChunk[chunkID]; ok {
		return l
	}
	l := &sync.Mutex{}
	s.lockByChunk[chunkID] = l
	return l
}

func (s *Service) getMeta(chunkID string) chunk.ChunkMeta {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	return s.chunkMeta[chunkID]
}

func (s *Service) setMeta(meta chunk.ChunkMeta) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	s.chunkMeta[meta.ChunkID] = meta
}

func (s *Service) writePrepare(chunkID string, data []byte, chainID uint32, chainVer, updateVer uint64) (chunk.ChunkMeta, error) {
	meta := s.getMeta(chunkID)
	if meta.UpdateVer > 0 && updateVer > 0 && updateVer > meta.UpdateVer+1 {
		return chunk.ChunkMeta{}, fmt.Errorf("missing update for %s: current=%d incoming=%d", chunkID, meta.UpdateVer, updateVer)
	}
	if updateVer == 0 {
		updateVer = meta.UpdateVer + 1
	}
	if meta.CommitVer >= updateVer {
		return meta, nil
	}
	if err := s.cfg.Chunks.Put(chunkID, data); err != nil {
		return chunk.ChunkMeta{}, err
	}
	meta = chunk.ChunkMeta{
		ChunkID:   chunkID,
		ChainID:   chainID,
		ChainVer:  chainVer,
		UpdateVer: updateVer,
		CommitVer: meta.CommitVer,
		State:     chunk.ChunkStatePending,
		UpdatedAt: time.Now(),
	}
	s.setMeta(meta)
	return meta, nil
}

func (s *Service) writeCommit(chunkID string, commitVer, chainVer uint64) chunk.ChunkMeta {
	meta := s.getMeta(chunkID)
	if commitVer == 0 {
		commitVer = meta.UpdateVer
	}
	if commitVer < meta.CommitVer {
		commitVer = meta.CommitVer
	}
	meta.CommitVer = commitVer
	meta.ChainVer = chainVer
	meta.State = chunk.ChunkStateCommitted
	meta.UpdatedAt = time.Now()
	s.setMeta(meta)
	return meta
}

func (s *Service) forwardReplica(ctx context.Context, nextNode, chunkID string, data []byte, req ReplicaWrite) error {
	return s.cfg.Transport.PutChunkWithOptions(ctx, nextNode, chunkID, data, transport.ChunkWriteOptions{
		Replica:    true,
		Stage:      req.Stage,
		ChainID:    req.ChainID,
		ChainVer:   req.ChainVer,
		UpdateVer:  req.UpdateVer,
		CommitVer:  req.CommitVer,
		Replicas:   req.Replicas,
		FromClient: false,
		Syncing:    req.Syncing,
	})
}

func (s *Service) idempotencyKey(chunkID string, req ReplicaWrite) string {
	return fmt.Sprintf("%s:%d:%d:%d:%s:%t", chunkID, req.ChainVer, req.UpdateVer, req.CommitVer, req.Stage, req.Syncing)
}

func (s *Service) rememberIdempotency(key string, st idempotencyState) {
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	s.idemState[key] = st
}

func (s *Service) lookupIdempotency(key string) (idempotencyState, bool) {
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	st, ok := s.idemState[key]
	return st, ok
}

func (s *Service) applyReplicaWrite(ctx context.Context, chunkID string, data []byte, req ReplicaWrite) error {
	lock := s.chunkLock(chunkID)
	lock.Lock()
	defer lock.Unlock()

	idKey := s.idempotencyKey(chunkID, req)
	if st, ok := s.lookupIdempotency(idKey); ok {
		if st.Err != "" {
			return fmt.Errorf("%s", st.Err)
		}
		return nil
	}

	if req.Stage == stageCommit {
		meta := s.writeCommit(chunkID, req.CommitVer, req.ChainVer)
		s.rememberIdempotency(idKey, idempotencyState{ChainVer: meta.ChainVer, UpdateVer: meta.UpdateVer, CommitVer: meta.CommitVer, At: time.Now()})
		return nil
	}
	if req.Stage == stageRemove {
		meta := s.getMeta(chunkID)
		nextUpdate := meta.UpdateVer + 1
		req.UpdateVer = nextUpdate
		req.CommitVer = nextUpdate
		if len(req.Replicas) == 0 {
			req.Replicas = []string{s.cfg.NodeHTTP}
		}
		nextNode := ""
		for i, node := range req.Replicas {
			if node == s.cfg.NodeHTTP && i+1 < len(req.Replicas) {
				nextNode = req.Replicas[i+1]
				break
			}
		}
		if nextNode != "" {
			if err := s.forwardReplica(ctx, nextNode, chunkID, nil, req); err != nil {
				s.rememberIdempotency(idKey, idempotencyState{Err: err.Error(), At: time.Now()})
				return err
			}
		}
		_ = s.cfg.Chunks.Delete(chunkID)
		meta = chunk.ChunkMeta{
			ChunkID:   chunkID,
			ChainID:   req.ChainID,
			ChainVer:  req.ChainVer,
			UpdateVer: nextUpdate,
			CommitVer: nextUpdate,
			State:     chunk.ChunkStateCommitted,
			UpdatedAt: time.Now(),
		}
		s.setMeta(meta)
		s.rememberIdempotency(idKey, idempotencyState{ChainVer: meta.ChainVer, UpdateVer: meta.UpdateVer, CommitVer: meta.CommitVer, At: time.Now()})
		return nil
	}

	meta, err := s.writePrepare(chunkID, data, req.ChainID, req.ChainVer, req.UpdateVer)
	if err != nil {
		s.rememberIdempotency(idKey, idempotencyState{Err: err.Error(), At: time.Now()})
		return err
	}
	req.UpdateVer = meta.UpdateVer
	req.CommitVer = meta.UpdateVer
	if len(req.Replicas) == 0 {
		req.Replicas = []string{s.cfg.NodeHTTP}
	}
	nextNode := ""
	for i, node := range req.Replicas {
		if node == s.cfg.NodeHTTP && i+1 < len(req.Replicas) {
			nextNode = req.Replicas[i+1]
			break
		}
	}
	if nextNode != "" {
		if err := s.forwardReplica(ctx, nextNode, chunkID, data, req); err != nil {
			s.rememberIdempotency(idKey, idempotencyState{Err: err.Error(), At: time.Now()})
			return err
		}
	}
	committed := s.writeCommit(chunkID, req.UpdateVer, req.ChainVer)
	s.rememberIdempotency(idKey, idempotencyState{ChainVer: committed.ChainVer, UpdateVer: committed.UpdateVer, CommitVer: committed.CommitVer, At: time.Now()})
	return nil
}
