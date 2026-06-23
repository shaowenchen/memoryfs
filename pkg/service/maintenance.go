package service

import (
	"context"
	"log"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/meta"
)

// Stats holds node storage statistics.
//
// MemBytes is the memory occupied by chunk payloads on this node and is
// primary storage in memoryfs (not a cache). Reads and writes hit the
// chunk store directly — there is no read-cache layer.
type Stats struct {
	ChunkCount     int    `json:"chunk_count"`
	DiskBytes      int64  `json:"disk_bytes"`
	DiskQuotaBytes int64  `json:"disk_quota_bytes"`
	MemBytes       int64  `json:"mem_bytes"`
	ReplicaFactor  int    `json:"replica_factor"`
	NodeState      string `json:"node_state"`
	ClusterEpoch   uint64 `json:"cluster_epoch"`
}

// Stats returns current node statistics.
func (s *Service) Stats() Stats {
	st := Stats{
		ChunkCount:    s.cfg.Chunks.Count(),
		ReplicaFactor: s.cfg.ReplicaFactor,
		NodeState:     string(s.cfg.Lifecycle.State()),
		ClusterEpoch:  s.syncClusterEpoch(),
	}
	if s.cfg.DiskQuotaGB > 0 {
		st.DiskQuotaBytes = s.cfg.DiskQuotaGB << 30
	}
	switch chunks := s.cfg.Chunks.(type) {
	case *chunk.QuotaDisk:
		if used, err := chunks.UsageBytes(); err == nil {
			st.DiskBytes = used
		}
	case *chunk.DiskStore:
		if used, err := chunks.UsageBytes(); err == nil {
			st.DiskBytes = used
		}
	case *chunk.QuotaMemory:
		st.MemBytes = chunks.UsageBytes()
	case *chunk.PreallocMemory:
		st.MemBytes = chunks.UsageBytes()
		if chunks.ReservedBytes() > 0 {
			st.DiskQuotaBytes = chunks.ReservedBytes()
		}
	case *chunk.MemoryStore:
		st.MemBytes = chunks.UsageBytes()
	}
	return st
}

// RunGC removes orphan chunks from local disk.
func (s *Service) RunGC(ctx context.Context) (int, error) {
	if s.cfg.Registry == nil {
		return 0, nil
	}
	indexed, err := s.cfg.Registry.ListIndexed()
	if err != nil {
		return 0, err
	}
	return chunk.GCOrphans(s.cfg.Chunks, indexed)
}

// MaintenanceConfig configures background maintenance.
type MaintenanceConfig struct {
	GCInterval    time.Duration
	FlushInterval time.Duration
	TTL           time.Duration
	DefaultTTL    time.Duration
}

// StartMaintenance runs periodic GC, disk flush, TTL expiry, and replica repair sweeps.
func (s *Service) StartMaintenance(ctx context.Context, cfg MaintenanceConfig) {
	tick := 30 * time.Second
	if cfg.FlushInterval > 0 && cfg.FlushInterval < tick {
		tick = cfg.FlushInterval
	}
	if cfg.GCInterval > 0 && cfg.GCInterval < tick {
		tick = cfg.GCInterval
	}
	go func() {
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		var lastFlush, lastGC time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				if cfg.FlushInterval > 0 && (lastFlush.IsZero() || now.Sub(lastFlush) >= cfg.FlushInterval) {
					if _, err := s.FlushChunksLogged(); err != nil {
						log.Printf("flush: %v", err)
					}
					lastFlush = now
				}
				if cfg.GCInterval > 0 && (lastGC.IsZero() || now.Sub(lastGC) >= cfg.GCInterval) {
					if n, err := s.RunGC(ctx); err != nil {
						log.Printf("gc: %v", err)
					} else if n > 0 {
						log.Printf("gc: removed %d orphan chunks", n)
					}
					lastGC = now
				}
				if cfg.TTL > 0 && s.IsLeader() {
					if n, err := s.ExpireFiles(ctx, cfg.TTL); err != nil {
						log.Printf("ttl: %v", err)
					} else if n > 0 {
						log.Printf("ttl: expired %d files", n)
					}
				}
				if n, f := s.RunRepair(ctx); n > 0 || f > 0 {
					log.Printf("repair: fixed=%d failed=%d pending=%d", n, f, s.RepairInfo(0).Pending)
				}
			}
		}
	}()
}

// ExpireFiles removes files past ExpireAt or older than maxAge.
func (s *Service) ExpireFiles(ctx context.Context, maxAge time.Duration) (int, error) {
	inos, err := s.cfg.Meta.ListInos(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	expired := 0
	for _, ino := range inos {
		if ino == meta.RootIno() {
			continue
		}
		attr, err := s.cfg.Meta.GetAttr(ctx, ino)
		if err != nil {
			continue
		}
		if attr.Mode&0o170000 == 0o040000 {
			continue
		}
		if attr.ExpireAt > 0 && now.Unix() > attr.ExpireAt {
			if err := s.expireFile(ctx, attr); err == nil {
				expired++
			}
			continue
		}
		if maxAge > 0 && attr.Mtime > 0 && now.Sub(time.Unix(attr.Mtime, 0)) > maxAge {
			if err := s.expireFile(ctx, attr); err == nil {
				expired++
			}
		}
	}
	return expired, nil
}

func (s *Service) expireFile(ctx context.Context, attr *meta.Attr) error {
	for _, id := range attr.Chunks {
		_ = s.cfg.Chunks.Delete(id)
		if s.cfg.Registry != nil {
			_ = s.DeleteChunkRegistry(ctx, id)
		}
	}
	return s.cfg.Meta.PurgeInode(ctx, attr.Ino)
}
