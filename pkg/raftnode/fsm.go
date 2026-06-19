package raftnode

import (
	"encoding/json"
	"io"

	"github.com/hashicorp/raft"

	"github.com/shaowenchen/memoryfs/pkg/kv"
)

// FSM implements raft.FSM backed by MemoryKV.
type FSM struct {
	kv *kv.MemoryKV
}

// NewFSM creates a raft FSM with a new memory KV store.
func NewFSM() *FSM {
	return &FSM{kv: kv.NewMemoryKV()}
}

// KV returns the underlying KV store.
func (f *FSM) KV() *kv.MemoryKV { return f.kv }

func (f *FSM) Apply(l *raft.Log) interface{} {
	if len(l.Data) == 0 {
		return nil
	}
	var batch []kv.Op
	if err := json.Unmarshal(l.Data, &batch); err == nil && len(batch) > 0 && batch[0].Type != "" {
		return f.kv.Batch(batch)
	}
	var op kv.Op
	if err := json.Unmarshal(l.Data, &op); err != nil {
		return err
	}
	return f.kv.Batch([]kv.Op{op})
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	data, err := f.kv.Snapshot()
	if err != nil {
		return nil, err
	}
	return &kvSnapshot{data: data}, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	return f.kv.Restore(data)
}

type kvSnapshot struct {
	data []byte
}

func (s *kvSnapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := sink.Write(s.data); err != nil {
		_ = sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *kvSnapshot) Release() {}
