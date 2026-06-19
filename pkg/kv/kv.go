package kv

import "errors"

var (
	ErrNotFound = errors.New("kv: key not found")
	ErrNilValue = errors.New("kv: nil value")
)

// OpType is a KV operation type used in batch/raft commands.
type OpType string

const (
	OpSet   OpType = "set"
	OpDel   OpType = "del"
	OpHSet  OpType = "hset"
	OpHDel  OpType = "hdel"
	OpIncr  OpType = "incr"
	OpSAdd  OpType = "sadd"
	OpSRem  OpType = "srem"
)

// Op is a single KV mutation.
type Op struct {
	Type   OpType `json:"type"`
	Key    string `json:"key"`
	Field  string `json:"field,omitempty"`
	Value  []byte `json:"value,omitempty"`
	Member string `json:"member,omitempty"`
}

// KV is a Redis-like embedded key-value store.
type KV interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Del(keys ...string) error
	Exists(key string) (bool, error)
	HGet(key, field string) ([]byte, error)
	HSet(key, field string, value []byte) error
	HGetAll(key string) (map[string][]byte, error)
	HDel(key string, fields ...string) error
	HExists(key, field string) (bool, error)
	HLen(key string) (int64, error)
	Incr(key string) (uint64, error)
	SAdd(key string, members ...string) error
	SRem(key string, members ...string) error
	SMembers(key string) ([]string, error)
	Batch(ops []Op) error
	Snapshot() ([]byte, error)
	Restore(data []byte) error
	Close() error
}
