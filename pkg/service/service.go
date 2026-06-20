package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/kv"
	"github.com/shaowenchen/memoryfs/pkg/lifecycle"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/raftnode"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

const configRFKey = "memoryfs:config:replica_factor"

// Config holds service dependencies.
type Config struct {
	NodeID        string
	NodeHTTP      string
	RaftNode      *raftnode.Node
	Meta          meta.Backend
	Chunks        chunk.Store
	Registry      *chunk.Registry
	Lifecycle     *lifecycle.Manager
	Transport     transport.ChunkTransport
	ReplicaFactor int
	DefaultTTL    time.Duration
	RepairQueue   *RepairQueue
}

// Service implements core MemoryFS node logic shared by HTTP and gRPC.
type Service struct {
	cfg Config
}

// New creates a service instance.
func New(cfg Config) *Service {
	if cfg.ReplicaFactor <= 0 {
		cfg.ReplicaFactor = chunk.DefaultReplicaFactor
	}
	if cfg.Lifecycle == nil {
		cfg.Lifecycle = lifecycle.NewManager()
	}
	if cfg.RepairQueue == nil {
		cfg.RepairQueue = NewRepairQueue()
	}
	return &Service{cfg: cfg}
}

func (s *Service) Meta() meta.Backend            { return s.cfg.Meta }
func (s *Service) Chunks() chunk.Store           { return s.cfg.Chunks }
func (s *Service) Lifecycle() *lifecycle.Manager { return s.cfg.Lifecycle }
func (s *Service) Raft() *raftnode.Node          { return s.cfg.RaftNode }
func (s *Service) IsLeader() bool                { return s.cfg.RaftNode.IsLeader() }
func (s *Service) ReplicaFactor() int            { return s.cfg.ReplicaFactor }

func (s *Service) LeaderHTTP() (string, error) {
	return s.cfg.RaftNode.LeaderHTTPAddr()
}

func (s *Service) ListNodes(ctx context.Context) ([]string, error) {
	return s.cfg.Meta.ListNodes(ctx)
}

func (s *Service) PersistReplicaFactor() error {
	if !s.IsLeader() {
		return nil
	}
	val := fmt.Sprintf("%d", s.cfg.ReplicaFactor)
	if rkv, ok := s.cfg.RaftNode.KV().(interface{ Set(string, []byte) error }); ok {
		return rkv.Set(configRFKey, []byte(val))
	}
	return s.cfg.RaftNode.KV().Set(configRFKey, []byte(val))
}

func (s *Service) Join(ctx context.Context, id, raftAddr, httpAddr, grpcAddr, rdmaAddr string) error {
	if !s.IsLeader() {
		return fmt.Errorf("not leader")
	}
	if err := s.cfg.RaftNode.AddVoter(id, raftAddr); err != nil {
		return err
	}
	ops := []kv.Op{
		{Type: kv.OpHSet, Key: raftnode.NodesKey(), Field: id, Value: []byte(httpAddr)},
		{Type: kv.OpSet, Key: raftnode.NodeHTTPKey(id), Value: []byte(httpAddr)},
		{Type: kv.OpSet, Key: raftnode.NodeRaftKey(id), Value: []byte(raftAddr)},
	}
	if grpcAddr != "" {
		ops = append(ops, kv.Op{Type: kv.OpSet, Key: nodeGRPCKey(id), Value: []byte(grpcAddr)})
	}
	if rdmaAddr != "" {
		ops = append(ops, kv.Op{Type: kv.OpSet, Key: nodeRDMAKey(id), Value: []byte(rdmaAddr)})
	}
	ops = append(ops, kv.Op{Type: kv.OpIncr, Key: clusterEpochKey})
	if rkv, ok := s.cfg.RaftNode.KV().(interface{ Batch([]kv.Op) error }); ok {
		if err := rkv.Batch(ops); err != nil {
			return err
		}
		s.syncClusterEpoch()
		return nil
	}
	return fmt.Errorf("kv batch not supported")
}

// RemoveNode removes a node from the cluster (leader only).
func (s *Service) RemoveNode(ctx context.Context, id string) error {
	if !s.IsLeader() {
		return fmt.Errorf("not leader")
	}
	raftData, err := s.cfg.RaftNode.KV().Get(raftnode.NodeRaftKey(id))
	if err != nil {
		return fmt.Errorf("node %s: %w", id, err)
	}
	if err := s.cfg.RaftNode.RemoveVoter(id); err != nil {
		return err
	}
	_ = raftData
	ops := []kv.Op{
		{Type: kv.OpHDel, Key: raftnode.NodesKey(), Field: id},
		{Type: kv.OpDel, Key: raftnode.NodeHTTPKey(id)},
		{Type: kv.OpDel, Key: raftnode.NodeRaftKey(id)},
		{Type: kv.OpDel, Key: nodeGRPCKey(id)},
		{Type: kv.OpDel, Key: nodeRDMAKey(id)},
		{Type: kv.OpIncr, Key: clusterEpochKey},
	}
	if rkv, ok := s.cfg.RaftNode.KV().(interface{ Batch([]kv.Op) error }); ok {
		if err := rkv.Batch(ops); err != nil {
			return err
		}
		s.syncClusterEpoch()
		return nil
	}
	return fmt.Errorf("kv batch not supported")
}

// Leave drains local chunks and requests removal from the cluster.
func (s *Service) Leave(ctx context.Context) (remaining int, drained bool, err error) {
	remaining, drained, err = s.Drain(ctx, false)
	if err != nil || !drained {
		return remaining, drained, err
	}
	leader, err := s.LeaderHTTP()
	if err != nil {
		return 0, true, err
	}
	if leader == s.cfg.NodeHTTP {
		if err := s.RemoveNode(ctx, s.cfg.NodeID); err != nil {
			return 0, true, err
		}
		return 0, true, nil
	}
	return 0, true, s.requestRemoveNode(ctx, leader, s.cfg.NodeID)
}

func (s *Service) requestRemoveNode(ctx context.Context, leader, id string) error {
	body := fmt.Sprintf(`{"id":"%s"}`, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(leader, "/")+"/v1/cluster/remove", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("remove node status %d", resp.StatusCode)
	}
	return nil
}

func nodeGRPCKey(id string) string { return "memoryfs:node:grpc:" + id }
func nodeRDMAKey(id string) string { return "memoryfs:node:rdma:" + id }

// StoreChunkLocal writes chunk to local disk/memory only (peer replication).
func (s *Service) StoreChunkLocal(chunkID string, data []byte) error {
	return s.cfg.Chunks.Put(chunkID, data)
}

// PutChunk stores a chunk locally and replicates to peer nodes.
func (s *Service) PutChunk(ctx context.Context, chunkID string, data []byte) ([]string, error) {
	if !s.cfg.Lifecycle.AcceptsChunks() {
		return nil, fmt.Errorf("node is draining")
	}
	nodes, err := s.cfg.Meta.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	replicas, err := chunk.SelectNodes(nodes, chunkID, s.cfg.ReplicaFactor)
	if err != nil {
		return nil, err
	}

	shouldStore := chunkContains(replicas, s.cfg.NodeHTTP)
	if shouldStore {
		if err := s.cfg.Chunks.Put(chunkID, data); err != nil {
			return nil, err
		}
	}

	replicated := 0
	for _, node := range replicas {
		if node == s.cfg.NodeHTTP {
			if shouldStore {
				replicated++
			}
			continue
		}
		if err := s.cfg.Transport.PutChunkReplica(ctx, node, chunkID, data); err != nil {
			log.Printf("replicate %s -> %s: %v", chunkID, node, err)
			continue
		}
		replicated++
	}

	if err := s.RecordChunkRegistry(ctx, chunkID, replicas); err != nil {
		return replicas, fmt.Errorf("registry: %w", err)
	}
	if replicated < s.cfg.ReplicaFactor {
		s.enqueueRepair(chunkID, replicas)
	}
	return replicas, nil
}

// GetChunk reads a chunk from local storage only.
func (s *Service) GetChunk(ctx context.Context, chunkID string) ([]byte, error) {
	if data, ok := s.cfg.Chunks.Get(chunkID); ok {
		return data, nil
	}
	return nil, fmt.Errorf("chunk not found")
}

// DeleteChunk removes a chunk locally and from registry.
func (s *Service) DeleteChunk(ctx context.Context, chunkID string) error {
	_ = s.cfg.Chunks.Delete(chunkID)
	return s.DeleteChunkRegistry(ctx, chunkID)
}

// Rebuild pulls missing chunks from peer replicas onto local disk.
func (s *Service) Rebuild(ctx context.Context) (int, error) {
	nodes, err := s.cfg.Meta.ListNodes(ctx)
	if err != nil {
		return 0, err
	}
	return chunk.Rebuild(ctx, s.cfg.NodeHTTP, s.cfg.ReplicaFactor, s.cfg.Chunks, s.cfg.Registry, nodes,
		func(ctx context.Context, nodeURL, chunkID string) ([]byte, error) {
			return s.cfg.Transport.GetChunk(ctx, nodeURL, chunkID)
		})
}

// Drain flushes local chunks to disk and ensures they are replicated before shutdown.
func (s *Service) Drain(ctx context.Context, force bool) (remaining int, drained bool, err error) {
	s.cfg.Lifecycle.StartDrain()
	if _, err := s.FlushChunksLogged(); err != nil {
		log.Printf("drain: flush warning: %v", err)
	}
	local := s.cfg.Chunks.List()
	nodes, _ := s.cfg.Meta.ListNodes(ctx)

	for _, chunkID := range local {
		data, ok := s.cfg.Chunks.Get(chunkID)
		if !ok {
			continue
		}
		replicas, err := chunk.SelectNodes(nodes, chunkID, s.cfg.ReplicaFactor)
		if err != nil {
			if !force {
				s.cfg.Lifecycle.SetPendingDrain(len(local))
				return len(local), false, err
			}
			continue
		}
		replicated := 0
		for _, node := range replicas {
			if node == s.cfg.NodeHTTP {
				replicated++
				continue
			}
			if err := s.cfg.Transport.PutChunkReplica(ctx, node, chunkID, data); err != nil {
				continue
			}
			replicated++
		}
		if replicated < s.cfg.ReplicaFactor && !force {
			s.cfg.Lifecycle.SetPendingDrain(len(local))
			return len(local), false, fmt.Errorf("chunk %s under-replicated (%d/%d)", chunkID, replicated, s.cfg.ReplicaFactor)
		}
		if err := s.RecordChunkRegistry(ctx, chunkID, replicas); err != nil {
			if !force {
				return len(local), false, err
			}
		}
	}

	s.cfg.Lifecycle.MarkDrained()
	return 0, true, nil
}

// Ready marks node active and rebuilds missing chunks from peers.
func (s *Service) Ready(ctx context.Context) {
	s.cfg.Lifecycle.Ready()
	n, err := s.Rebuild(ctx)
	if err != nil {
		log.Printf("rebuild warning: %v", err)
	} else if n > 0 {
		log.Printf("rebuilt %d chunks from peer replicas", n)
	}
}

// Health returns node health info.
func (s *Service) Health() (status, state, role string, epoch uint64, pending int) {
	epoch = s.syncClusterEpoch()
	role = "follower"
	if s.IsLeader() {
		role = "leader"
	}
	return "ok", string(s.cfg.Lifecycle.State()), role, epoch, s.cfg.Lifecycle.PendingDrain()
}

func chunkContains(replicas []string, node string) bool {
	for _, r := range replicas {
		if r == node {
			return true
		}
	}
	return false
}

// FS operations delegate to meta backend.
func (s *Service) GetAttr(ctx context.Context, ino uint64) (*meta.Attr, error) {
	return s.cfg.Meta.GetAttr(ctx, ino)
}
func (s *Service) Lookup(ctx context.Context, parentIno uint64, name string) (*meta.Attr, error) {
	return s.cfg.Meta.Lookup(ctx, parentIno, name)
}
func (s *Service) Readdir(ctx context.Context, parentIno uint64) (map[string]*meta.Attr, error) {
	return s.cfg.Meta.Readdir(ctx, parentIno)
}
func (s *Service) Mkdir(ctx context.Context, parentIno uint64, name string, mode, uid, gid uint32) (*meta.Attr, error) {
	return s.cfg.Meta.Mkdir(ctx, parentIno, name, mode, uid, gid)
}
func (s *Service) Create(ctx context.Context, parentIno uint64, name string, mode, uid, gid uint32) (*meta.Attr, error) {
	attr, err := s.cfg.Meta.Create(ctx, parentIno, name, mode, uid, gid)
	if err != nil {
		return nil, err
	}
	if s.cfg.DefaultTTL > 0 {
		attr.ExpireAt = time.Now().Add(s.cfg.DefaultTTL).Unix()
		if err := s.cfg.Meta.UpdateAttr(ctx, attr); err != nil {
			return attr, err
		}
	}
	return attr, nil
}
func (s *Service) Symlink(ctx context.Context, parentIno uint64, name, target string, uid, gid uint32) (*meta.Attr, error) {
	return s.cfg.Meta.Symlink(ctx, parentIno, name, target, uid, gid)
}
func (s *Service) Unlink(ctx context.Context, parentIno uint64, name string) (*meta.Attr, error) {
	return s.cfg.Meta.Unlink(ctx, parentIno, name)
}
func (s *Service) Rmdir(ctx context.Context, parentIno uint64, name string) error {
	return s.cfg.Meta.Rmdir(ctx, parentIno, name)
}
func (s *Service) Rename(ctx context.Context, oldParent, newParent uint64, oldName, newName string) error {
	return s.cfg.Meta.Rename(ctx, oldParent, newParent, oldName, newName)
}
func (s *Service) SetAttr(ctx context.Context, attr *meta.Attr) error {
	return s.cfg.Meta.UpdateAttr(ctx, attr)
}
