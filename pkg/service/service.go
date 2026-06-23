package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/cluster"
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
	URIPrefix     string
	RaftNode      *raftnode.Node
	Meta          meta.Backend
	Chunks        chunk.Store
	Registry      *chunk.Registry
	Lifecycle     *lifecycle.Manager
	Transport     transport.ChunkTransport
	ReplicaFactor int
	DefaultTTL    time.Duration
	RepairQueue   *RepairQueue
	DiskQuotaGB   int64
	Membership    *cluster.Membership
}

// Service implements core MemoryFS node logic shared by HTTP and gRPC.
type Service struct {
	cfg          Config
	metaMu       sync.Mutex
	chunkMeta    map[string]chunk.ChunkMeta
	idemMu       sync.Mutex
	idemState    map[string]idempotencyState
	lockMu       sync.Mutex
	lockByChunk  map[string]*sync.Mutex
	stateMu      sync.Mutex
	chainStates  map[uint32]chunk.PublicTargetState
	chainVers    map[uint32]uint64
	syncingChain map[uint32]bool
}

type idempotencyState struct {
	ChainVer  uint64
	UpdateVer uint64
	CommitVer uint64
	Err       string
	At        time.Time
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
	if cfg.Membership == nil && cfg.RaftNode != nil {
		cfg.Membership = cluster.NewMembership(cfg.RaftNode, cfg.NodeID)
	}
	return &Service{
		cfg:        cfg,
		chunkMeta:  make(map[string]chunk.ChunkMeta),
		idemState:  make(map[string]idempotencyState),
		lockByChunk: make(map[string]*sync.Mutex),
		chainStates: make(map[uint32]chunk.PublicTargetState),
		chainVers: make(map[uint32]uint64),
		syncingChain: make(map[uint32]bool),
	}
}

func (s *Service) Membership() *cluster.Membership { return s.cfg.Membership }

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
	if s.cfg.Membership != nil {
		return s.cfg.Membership.ListHTTP(ctx)
	}
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

func (s *Service) Join(ctx context.Context, id, raftAddr, httpAddr, grpcAddr, rdmaAddr string) ([]string, error) {
	if s.cfg.Membership == nil {
		return nil, fmt.Errorf("membership not configured")
	}
	members, err := s.cfg.Membership.Admit(ctx, cluster.Member{
		ID: id, Raft: raftAddr, HTTP: httpAddr, GRPC: grpcAddr, RDMA: rdmaAddr,
	})
	if err != nil {
		return nil, err
	}
	s.syncClusterEpoch()
	s.recomputeChainStates(ctx)
	return memberHTTPURLs(members), nil
}

func memberHTTPURLs(members []cluster.Member) []string {
	out := make([]string, 0, len(members))
	for _, m := range members {
		if m.HTTP != "" {
			out = append(out, m.HTTP)
		}
	}
	return out
}

// RemoveNode removes a node from the cluster (leader only).
func (s *Service) RemoveNode(ctx context.Context, id string) error {
	if s.cfg.Membership == nil {
		return fmt.Errorf("membership not configured")
	}
	if err := s.cfg.Membership.Remove(ctx, id); err != nil {
		return err
	}
	s.syncClusterEpoch()
	s.recomputeChainStates(ctx)
	return nil
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

// GetChunkRegistry returns replica node URLs for a chunk from the registry.
func (s *Service) GetChunkRegistry(_ context.Context, chunkID string) (*chunk.Location, error) {
	if s.cfg.Registry == nil {
		return nil, fmt.Errorf("no registry")
	}
	return s.cfg.Registry.Get(chunkID)
}

// StoreChunkLocal applies a CRAQ replica write from a predecessor target.
func (s *Service) StoreChunkLocal(ctx context.Context, chunkID string, data []byte, req ReplicaWrite) error {
	return s.applyReplicaWrite(ctx, chunkID, data, req)
}

// PutChunk stores a chunk locally and replicates to all peer replicas.
func (s *Service) PutChunk(ctx context.Context, chunkID string, data []byte) ([]string, error) {
	return s.putChunk(ctx, chunkID, data, true)
}

// putChunk implements strict CRAQ: only HEAD accepts client writes, then
// synchronously forwards along the chain and commits on ACK.
func (s *Service) putChunk(ctx context.Context, chunkID string, data []byte, replicatePeers bool) ([]string, error) {
	if !s.cfg.Lifecycle.AcceptsChunks() {
		return nil, fmt.Errorf("node is draining")
	}
	nodes, err := s.cfg.Meta.ListNodes(ctx)
	if err != nil || len(nodes) == 0 {
		log.Printf("putChunk %s bytes=%d cluster not ready (nodes=%d err=%v), storing locally", chunkID, len(data), len(nodes), err)
		if putErr := s.cfg.Chunks.Put(chunkID, data); putErr != nil {
			return nil, putErr
		}
		s.enqueueRepair(chunkID, []string{s.cfg.NodeHTTP})
		return []string{s.cfg.NodeHTTP}, nil
	}
	chain, err := chunk.ChainFor(nodes, chunkID, s.cfg.ReplicaFactor)
	if err != nil {
		return nil, err
	}
	replicas := chain.NodeURLs()

	head := chain.Head().NodeURL
	if head != s.cfg.NodeHTTP {
		return nil, fmt.Errorf("routing error: node %s is not chain head %s", s.cfg.NodeHTTP, head)
	}
	req := ReplicaWrite{
		Stage:      stagePrepare,
		ChainID:    chain.ID,
		ChainVer:   s.chainVersion(chain.ID),
		Replicas:   replicas,
		FromClient: true,
	}
	if err := s.applyReplicaWrite(ctx, chunkID, data, req); err != nil {
		return nil, err
	}
	log.Printf("putChunk %s bytes=%d chain=%d head=%s targets=%v strict=true", chunkID, len(data), chain.ID, s.cfg.NodeHTTP, replicas)
	if replicatePeers {
		if err := s.RecordChunkRegistry(context.Background(), chunkID, replicas); err != nil {
			log.Printf("putChunk %s registry deferred: %v", chunkID, err)
			s.enqueueRepair(chunkID, replicas)
		}
	}
	return replicas, nil
}

// GetChunk reads a chunk from local storage only.
func (s *Service) GetChunk(ctx context.Context, chunkID string) ([]byte, error) {
	meta := s.getMeta(chunkID)
	if !meta.Committed() && meta.UpdateVer != 0 {
		return nil, fmt.Errorf("chunk not committed")
	}
	if data, ok := s.cfg.Chunks.Get(chunkID); ok {
		return data, nil
	}
	return nil, fmt.Errorf("chunk not found")
}

// GetChunkWithVisibility reads a chunk with optional relaxed visibility.
func (s *Service) GetChunkWithVisibility(ctx context.Context, chunkID string, allowUncommitted bool) ([]byte, error) {
	if !allowUncommitted {
		return s.GetChunk(ctx, chunkID)
	}
	if data, ok := s.cfg.Chunks.Get(chunkID); ok {
		return data, nil
	}
	return nil, fmt.Errorf("chunk not found")
}

// DeleteChunk removes a chunk locally and from registry.
func (s *Service) DeleteChunk(ctx context.Context, chunkID string) error {
	nodes, err := s.cfg.Meta.ListNodes(ctx)
	if err != nil || len(nodes) == 0 {
		meta := s.getMeta(chunkID)
		nextVer := meta.UpdateVer + 1
		_ = s.cfg.Chunks.Delete(chunkID)
		s.setMeta(chunk.ChunkMeta{
			ChunkID:   chunkID,
			ChainID:   meta.ChainID,
			ChainVer:  s.syncClusterEpoch(),
			UpdateVer: nextVer,
			CommitVer: nextVer,
			State:     chunk.ChunkStateCommitted,
			UpdatedAt: time.Now(),
		})
		return s.DeleteChunkRegistry(ctx, chunkID)
	}
	chain, err := chunk.ChainFor(nodes, chunkID, s.cfg.ReplicaFactor)
	if err != nil {
		return err
	}
	head := chain.Head().NodeURL
	if head != s.cfg.NodeHTTP {
		return fmt.Errorf("routing error: node %s is not chain head %s", s.cfg.NodeHTTP, head)
	}
	if err := s.applyReplicaWrite(ctx, chunkID, nil, ReplicaWrite{
		Stage:      stageRemove,
		ChainID:    chain.ID,
		ChainVer:   s.chainVersion(chain.ID),
		Replicas:   chain.NodeURLs(),
		FromClient: true,
	}); err != nil {
		return err
	}
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
	s.recomputeChainStates(ctx)
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
	s.recomputeChainStates(context.Background())
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
