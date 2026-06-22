package service

import (
	"context"
	"fmt"
	"log"
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
	replicated := 0
	for _, node := range replicas {
		if node == s.cfg.NodeHTTP {
			replicated++
			continue
		}
		if err := s.cfg.Transport.PutChunkReplica(ctx, node, chunkID, data); err != nil {
			log.Printf("repair %s -> %s: %v", chunkID, node, err)
			continue
		}
		replicated++
	}
	if replicated < s.cfg.ReplicaFactor {
		return fmt.Errorf("under-replicated (%d/%d)", replicated, s.cfg.ReplicaFactor)
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
