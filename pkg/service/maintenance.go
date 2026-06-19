package service

import (
	"context"
	"log"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/meta"
)

// Stats holds node storage statistics.
type Stats struct {
	ChunkCount    int    `json:"chunk_count"`
	DiskBytes     int64  `json:"disk_bytes"`
	MemCacheBytes int64  `json:"mem_cache_bytes"`
	ReplicaFactor int    `json:"replica_factor"`
	NodeState     string `json:"node_state"`
	ClusterEpoch  uint64 `json:"cluster_epoch"`
}

// Stats returns current node statistics.
func (s *Service) Stats() Stats {
	st := Stats{
		ChunkCount:    s.cfg.Chunks.Count(),
		ReplicaFactor: s.cfg.ReplicaFactor,
		NodeState:     string(s.cfg.Lifecycle.State()),
		ClusterEpoch:  s.cfg.Lifecycle.Epoch(),
	}
	if ts, ok := s.cfg.Chunks.(*chunk.TieredStore); ok {
		st.MemCacheBytes = ts.MemUsage()
		if used, err := ts.DiskUsage(); err == nil {
			st.DiskBytes = used
		}
	} else if ds, ok := s.cfg.Chunks.(*chunk.QuotaDisk); ok {
		if used, err := ds.UsageBytes(); err == nil {
			st.DiskBytes = used
		}
	} else if ds, ok := s.cfg.Chunks.(*chunk.DiskStore); ok {
		if used, err := ds.UsageBytes(); err == nil {
			st.DiskBytes = used
		}
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
	GCInterval time.Duration
	TTL        time.Duration
	DefaultTTL time.Duration
}

// StartMaintenance runs periodic GC and TTL expiry sweeps.
func (s *Service) StartMaintenance(ctx context.Context, cfg MaintenanceConfig) {
	if cfg.GCInterval <= 0 {
		cfg.GCInterval = 5 * time.Minute
	}
	go func() {
		ticker := time.NewTicker(cfg.GCInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := s.RunGC(ctx); err != nil {
					log.Printf("gc: %v", err)
				} else if n > 0 {
					log.Printf("gc: removed %d orphan chunks", n)
				}
				if cfg.TTL > 0 && s.IsLeader() {
					if n, err := s.ExpireFiles(ctx, cfg.TTL); err != nil {
						log.Printf("ttl: %v", err)
					} else if n > 0 {
						log.Printf("ttl: expired %d files", n)
					}
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
			_ = s.cfg.Registry.Delete(id)
		}
	}
	return s.cfg.Meta.PurgeInode(ctx, attr.Ino)
}
