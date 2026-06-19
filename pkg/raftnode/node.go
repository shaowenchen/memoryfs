package raftnode

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	"github.com/shaowenchen/memoryfs/pkg/kv"
)

// Config holds raft node configuration.
type Config struct {
	ID            string
	RaftAddr      string
	AdvertiseRaft string
	DataDir       string
	Bootstrap     bool
	Standalone    bool
	JoinAddr      string
	HTTPAddr      string
}

// Node wraps a raft instance and its FSM.
type Node struct {
	raft      *raft.Raft
	fsm       *FSM
	kv        kv.KV
	boltStore *raftboltdb.BoltStore
	config    Config
}

// Start creates and starts a raft node or standalone KV node.
func Start(cfg Config) (*Node, error) {
	if cfg.Standalone {
		fsm := NewFSM()
		return &Node{
			fsm:    fsm,
			kv:     fsm.KV(),
			config: cfg,
		}, nil
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}

	fsm := NewFSM()
	snapDir := filepath.Join(cfg.DataDir, "snapshots")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return nil, err
	}

	snapStore, err := raft.NewFileSnapshotStore(snapDir, 2, os.Stderr)
	if err != nil {
		return nil, err
	}
	raftDBPath := filepath.Join(cfg.DataDir, "raft.db")
	boltStore, err := raftboltdb.NewBoltStore(raftDBPath)
	if err != nil {
		return nil, err
	}
	logStore := boltStore
	stableStore := boltStore

	transport, err := newTransport(cfg.RaftAddr)
	if err != nil {
		return nil, err
	}

	raftCfg := raft.DefaultConfig()
	raftCfg.LocalID = raft.ServerID(cfg.ID)
	raftCfg.SnapshotInterval = 30 * time.Second
	raftCfg.SnapshotThreshold = 128

	r, err := raft.NewRaft(raftCfg, fsm, logStore, stableStore, snapStore, transport)
	if err != nil {
		return nil, err
	}

	node := &Node{
		raft:      r,
		fsm:       fsm,
		kv:        kv.NewRaftKV(r, fsm.KV()),
		boltStore: boltStore,
		config:    cfg,
	}

	if cfg.Bootstrap {
		raftAddr := raft.ServerAddress(cfg.AdvertiseRaft)
		if raftAddr == "" {
			raftAddr = transport.LocalAddr()
		}
		configuration := raft.Configuration{
			Servers: []raft.Server{{
				ID:      raft.ServerID(cfg.ID),
				Address: raftAddr,
			}},
		}
		if err := r.BootstrapCluster(configuration).Error(); err != nil {
			return nil, fmt.Errorf("bootstrap: %w", err)
		}
	}

	return node, nil
}

func newTransport(addr string) (*raft.NetworkTransport, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return raft.NewNetworkTransport(NewStreamLayer(ln), 3, 10*time.Second, os.Stderr), nil
}

// StreamLayer adapts a net.Listener for raft transport.
type StreamLayer struct {
	ln net.Listener
}

// NewStreamLayer wraps a listener for raft.
func NewStreamLayer(ln net.Listener) *StreamLayer {
	return &StreamLayer{ln: ln}
}

func (s *StreamLayer) Accept() (net.Conn, error) { return s.ln.Accept() }
func (s *StreamLayer) Close() error              { return s.ln.Close() }
func (s *StreamLayer) Addr() net.Addr            { return s.ln.Addr() }
func (s *StreamLayer) Dial(addr raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", string(addr), timeout)
}

// KV returns the metadata KV store.
func (n *Node) KV() kv.KV { return n.kv }

// IsLeader reports whether this node is the raft leader.
func (n *Node) IsLeader() bool {
	if n.raft == nil {
		return true
	}
	return n.raft.State() == raft.Leader
}

// LeaderHTTPAddr returns the leader's HTTP address stored in KV.
func (n *Node) LeaderHTTPAddr() (string, error) {
	if n.IsLeader() {
		return n.config.HTTPAddr, nil
	}
	if n.raft == nil {
		return n.config.HTTPAddr, nil
	}
	addr, id := n.raft.LeaderWithID()
	if addr == "" {
		return "", fmt.Errorf("no leader elected")
	}
	data, err := n.kv.Get(nodeHTTPKey(string(id)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AddVoter adds a new node to the raft cluster.
func (n *Node) AddVoter(id, raftAddr string) error {
	if n.raft == nil {
		return fmt.Errorf("standalone mode")
	}
	if !n.IsLeader() {
		return fmt.Errorf("not leader")
	}
	future := n.raft.AddVoter(raft.ServerID(id), raft.ServerAddress(raftAddr), 0, 0)
	return future.Error()
}

// RemoveVoter removes a node from the raft cluster.
func (n *Node) RemoveVoter(id string) error {
	if n.raft == nil {
		return fmt.Errorf("standalone mode")
	}
	if !n.IsLeader() {
		return fmt.Errorf("not leader")
	}
	future := n.raft.RemoveServer(raft.ServerID(id), 0, 0)
	return future.Error()
}

// RegisterSelf stores this node's HTTP and raft addresses in KV.
func (n *Node) RegisterSelf() error {
	raftAddr := n.config.AdvertiseRaft
	if raftAddr == "" {
		raftAddr = n.config.RaftAddr
	}
	ops := []kv.Op{
		{Type: kv.OpHSet, Key: nodesKey(), Field: n.config.ID, Value: []byte(n.config.HTTPAddr)},
		{Type: kv.OpSet, Key: nodeHTTPKey(n.config.ID), Value: []byte(n.config.HTTPAddr)},
		{Type: kv.OpSet, Key: nodeRaftKey(n.config.ID), Value: []byte(raftAddr)},
	}
	if n.raft != nil {
		rkv, ok := n.kv.(*kv.RaftKV)
		if !ok {
			return fmt.Errorf("unexpected kv type")
		}
		return rkv.Batch(ops)
	}
	return n.kv.(*kv.MemoryKV).Batch(ops)
}

// Close shuts down the raft node.
func (n *Node) Close() error {
	if n.raft != nil {
		future := n.raft.Shutdown()
		if err := future.Error(); err != nil {
			return err
		}
	}
	if n.boltStore != nil {
		return n.boltStore.Close()
	}
	return nil
}

func nodesKey() string { return "memoryfs:nodes" }

func nodeHTTPKey(id string) string { return "memoryfs:node:http:" + id }

func nodeRaftKey(id string) string { return "memoryfs:node:raft:" + id }

// ListNodeHTTPAddrs returns registered node HTTP URLs.
func ListNodeHTTPAddrs(store kv.KV) ([]string, error) {
	fields, err := store.HGetAll(nodesKey())
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(fields))
	for _, v := range fields {
		out = append(out, string(v))
	}
	return out, nil
}

// NodesKey is exported for tests.
func NodesKey() string { return nodesKey() }

func NodeHTTPKey(id string) string { return nodeHTTPKey(id) }

func NodeRaftKey(id string) string { return nodeRaftKey(id) }
