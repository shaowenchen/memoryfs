package kv

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/raft"
)

const raftTimeout = 10 * time.Second

// RaftKV applies mutations through Raft and reads from the local FSM.
type RaftKV struct {
	raft *raft.Raft
	fsm  *MemoryKV
}

// NewRaftKV wraps a raft instance and its FSM memory store.
func NewRaftKV(r *raft.Raft, fsm *MemoryKV) *RaftKV {
	return &RaftKV{raft: r, fsm: fsm}
}

func (r *RaftKV) apply(op Op) error {
	if r.raft.State() != raft.Leader {
		return fmt.Errorf("not leader")
	}
	data, err := json.Marshal(op)
	if err != nil {
		return err
	}
	future := r.raft.Apply(data, raftTimeout)
	return future.Error()
}

func (r *RaftKV) applyBatch(ops []Op) error {
	if r.raft.State() != raft.Leader {
		return fmt.Errorf("not leader")
	}
	data, err := json.Marshal(ops)
	if err != nil {
		return err
	}
	future := r.raft.Apply(data, raftTimeout)
	return future.Error()
}

func (r *RaftKV) Set(key string, value []byte) error {
	return r.apply(Op{Type: OpSet, Key: key, Value: value})
}

func (r *RaftKV) Del(keys ...string) error {
	for _, key := range keys {
		if err := r.apply(Op{Type: OpDel, Key: key}); err != nil {
			return err
		}
	}
	return nil
}

func (r *RaftKV) HSet(key, field string, value []byte) error {
	return r.apply(Op{Type: OpHSet, Key: key, Field: field, Value: value})
}

func (r *RaftKV) HDel(key string, fields ...string) error {
	for _, field := range fields {
		if err := r.apply(Op{Type: OpHDel, Key: key, Field: field}); err != nil {
			return err
		}
	}
	return nil
}

func (r *RaftKV) Incr(key string) (uint64, error) {
	if err := r.apply(Op{Type: OpIncr, Key: key}); err != nil {
		return 0, err
	}
	data, err := r.fsm.Get(key)
	if err != nil {
		return 0, err
	}
	var v uint64
	_, err = fmt.Sscanf(string(data), "%d", &v)
	return v, err
}

func (r *RaftKV) SAdd(key string, members ...string) error {
	for _, member := range members {
		if err := r.apply(Op{Type: OpSAdd, Key: key, Member: member}); err != nil {
			return err
		}
	}
	return nil
}

func (r *RaftKV) SRem(key string, members ...string) error {
	for _, member := range members {
		if err := r.apply(Op{Type: OpSRem, Key: key, Member: member}); err != nil {
			return err
		}
	}
	return nil
}

func (r *RaftKV) Batch(ops []Op) error {
	if len(ops) == 0 {
		return nil
	}
	return r.applyBatch(ops)
}

func (r *RaftKV) Get(key string) ([]byte, error)             { return r.fsm.Get(key) }
func (r *RaftKV) Exists(key string) (bool, error)            { return r.fsm.Exists(key) }
func (r *RaftKV) HGet(key, field string) ([]byte, error)     { return r.fsm.HGet(key, field) }
func (r *RaftKV) HGetAll(key string) (map[string][]byte, error) { return r.fsm.HGetAll(key) }
func (r *RaftKV) HExists(key, field string) (bool, error)    { return r.fsm.HExists(key, field) }
func (r *RaftKV) HLen(key string) (int64, error)             { return r.fsm.HLen(key) }
func (r *RaftKV) SMembers(key string) ([]string, error)      { return r.fsm.SMembers(key) }
func (r *RaftKV) Snapshot() ([]byte, error)                  { return r.fsm.Snapshot() }
func (r *RaftKV) Restore(data []byte) error                    { return r.fsm.Restore(data) }
func (r *RaftKV) Close() error                                 { return nil }

// IsLeader reports whether this node is the current raft leader.
func (r *RaftKV) IsLeader() bool { return r.raft.State() == raft.Leader }

// Raft returns the underlying raft instance.
func (r *RaftKV) Raft() *raft.Raft { return r.raft }
