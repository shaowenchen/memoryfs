package cluster

import (
	"fmt"

	"github.com/shaowenchen/memoryfs/pkg/kv"
	"github.com/shaowenchen/memoryfs/pkg/raftnode"
)

func registerOps(m Member) []kv.Op {
	ops := []kv.Op{
		{Type: kv.OpHSet, Key: raftnode.NodesKey(), Field: m.ID, Value: []byte(m.HTTP)},
		{Type: kv.OpSet, Key: raftnode.NodeHTTPKey(m.ID), Value: []byte(m.HTTP)},
		{Type: kv.OpSet, Key: raftnode.NodeRaftKey(m.ID), Value: []byte(m.Raft)},
	}
	if m.GRPC != "" {
		ops = append(ops, kv.Op{Type: kv.OpSet, Key: nodeGRPCKey(m.ID), Value: []byte(m.GRPC)})
	}
	if m.RDMA != "" {
		ops = append(ops, kv.Op{Type: kv.OpSet, Key: nodeRDMAKey(m.ID), Value: []byte(m.RDMA)})
	}
	return ops
}

func removeOps(id string) []kv.Op {
	return []kv.Op{
		{Type: kv.OpHDel, Key: raftnode.NodesKey(), Field: id},
		{Type: kv.OpDel, Key: raftnode.NodeHTTPKey(id)},
		{Type: kv.OpDel, Key: raftnode.NodeRaftKey(id)},
		{Type: kv.OpDel, Key: nodeGRPCKey(id)},
		{Type: kv.OpDel, Key: nodeRDMAKey(id)},
	}
}

func batch(store kv.KV, ops []kv.Op) error {
	if b, ok := store.(interface{ Batch([]kv.Op) error }); ok {
		return b.Batch(ops)
	}
	return fmt.Errorf("kv batch not supported")
}

func listHTTP(store kv.KV) ([]string, error) {
	return raftnode.ListNodeHTTPAddrs(store)
}

func listMembers(store kv.KV) ([]Member, error) {
	fields, err := store.HGetAll(raftnode.NodesKey())
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(fields))
	for id, http := range fields {
		m := Member{ID: id, HTTP: string(http)}
		if raftAddr, err := store.Get(raftnode.NodeRaftKey(id)); err == nil {
			m.Raft = string(raftAddr)
		}
		if grpcAddr, err := store.Get(nodeGRPCKey(id)); err == nil {
			m.GRPC = string(grpcAddr)
		}
		if rdmaAddr, err := store.Get(nodeRDMAKey(id)); err == nil {
			m.RDMA = string(rdmaAddr)
		}
		out = append(out, m)
	}
	return out, nil
}
