package service

import (
	"context"
	"fmt"
	"log"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
)

// RunRepair processes pending replica repair jobs.
func (s *Service) RunRepair(ctx context.Context) (repaired, failed int) {
	if s.cfg.RepairQueue == nil {
		return 0, 0
	}
	for _, job := range s.cfg.RepairQueue.Snapshot() {
		if err := s.repairChunk(ctx, job.ChunkID, job.Replicas); err != nil {
			s.cfg.RepairQueue.Fail(job.ChunkID, err.Error())
			failed++
			continue
		}
		s.cfg.RepairQueue.Done(job.ChunkID)
		repaired++
	}
	return repaired, failed
}

func (s *Service) repairChunk(ctx context.Context, chunkID string, replicas []string) error {
	data, ok := s.cfg.Chunks.Get(chunkID)
	if !ok {
		var err error
		data, err = s.fetchChunkFromPeers(ctx, chunkID, replicas)
		if err != nil {
			return err
		}
	}

	selfInReplicas := chunkContains(replicas, s.cfg.NodeHTTP)
	if selfInReplicas {
		nodes, _ := s.cfg.Meta.ListNodes(ctx)
		chainID := uint32(0)
		chainVer := s.syncClusterEpoch()
		if chain, err := chunk.ChainFor(nodes, chunkID, s.cfg.ReplicaFactor); err == nil {
			chainID = chain.ID
			chainVer = s.chainVersion(chainID)
		}
		if _, err := s.SyncStart(ctx, chainID, chainVer); err != nil {
			return err
		}
		defer func() {
			_ = s.SyncDone(context.Background(), chainID, chainVer)
		}()
		if err := s.applyReplicaWrite(ctx, chunkID, data, ReplicaWrite{
			Stage:    stagePrepare,
			ChainID:  chainID,
			ChainVer: chainVer,
			Replicas: replicas,
			Syncing:  true,
		}); err != nil {
			return err
		}
	}
	if err := s.RecordChunkRegistry(ctx, chunkID, replicas); err != nil {
		return err
	}
	if !selfInReplicas {
		log.Printf("repair %s: transit copy removed (not in replica set %v)", chunkID, replicas)
		_ = s.cfg.Chunks.Delete(chunkID)
	}
	return nil
}

func (s *Service) fetchChunkFromPeers(ctx context.Context, chunkID string, replicas []string) ([]byte, error) {
	for _, node := range replicas {
		if node == s.cfg.NodeHTTP {
			continue
		}
		data, err := s.cfg.Transport.GetChunk(ctx, node, chunkID)
		if err != nil {
			continue
		}
		_ = s.cfg.Chunks.Put(chunkID, data)
		return data, nil
	}
	return nil, fmt.Errorf("no peer replica for %s", chunkID)
}

func (s *Service) enqueueRepair(chunkID string, replicas []string) {
	if s.cfg.RepairQueue != nil {
		s.cfg.RepairQueue.Enqueue(chunkID, replicas)
	}
}

func (s *Service) RepairInfo(limit int) RepairInfo {
	if s.cfg.RepairQueue == nil {
		return RepairInfo{}
	}
	return s.cfg.RepairQueue.Info(limit)
}
