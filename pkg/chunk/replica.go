package chunk

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/shaowenchen/memoryfs/pkg/kv"
)

const (
	// DefaultReplicaFactor is the default chunk replication factor.
	DefaultReplicaFactor = 2
	chunkLocPrefix       = "memoryfs:chunkloc:"
)

// Location records where a chunk is replicated.
type Location struct {
	ChunkID  string   `json:"chunk_id"`
	Replicas []string `json:"replicas"`
	Epoch    uint64   `json:"epoch"`
}

// Registry tracks chunk replica locations in KV.
type Registry struct {
	kv kv.KV
}

// NewRegistry creates a chunk location registry.
func NewRegistry(store kv.KV) *Registry {
	return &Registry{kv: store}
}

func chunkLocKey(id string) string { return chunkLocPrefix + id }

// Set stores replica locations for a chunk.
func (r *Registry) Set(id string, replicas []string, epoch uint64) error {
	loc := Location{ChunkID: id, Replicas: replicas, Epoch: epoch}
	data, err := json.Marshal(loc)
	if err != nil {
		return err
	}
	if err := r.kv.Set(chunkLocKey(id), data); err != nil {
		return err
	}
	return r.kv.SAdd(chunkIndexKey, id)
}

// Get returns replica locations for a chunk.
func (r *Registry) Get(id string) (*Location, error) {
	data, err := r.kv.Get(chunkLocKey(id))
	if err != nil {
		return nil, err
	}
	var loc Location
	if err := json.Unmarshal(data, &loc); err != nil {
		return nil, err
	}
	return &loc, nil
}

// Delete removes chunk location metadata.
func (r *Registry) Delete(id string) error {
	_ = r.kv.SRem(chunkIndexKey, id)
	return r.kv.Del(chunkLocKey(id))
}

// ListIndexed returns all known chunk ids.
func (r *Registry) ListIndexed() ([]string, error) {
	return r.kv.SMembers(chunkIndexKey)
}

// SelectNodes picks primary and replica nodes for a chunk.
func SelectNodes(nodes []string, chunkID string, rf int) ([]string, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}
	if rf <= 0 {
		rf = 1
	}
	if rf > len(nodes) {
		rf = len(nodes)
	}
	sorted := append([]string(nil), nodes...)
	sort.Strings(sorted)

	h := fnv.New32a()
	_, _ = h.Write([]byte(chunkID))
	start := int(h.Sum32() % uint32(len(sorted)))

	out := make([]string, 0, rf)
	for i := 0; i < rf; i++ {
		out = append(out, sorted[(start+i)%len(sorted)])
	}
	return out, nil
}
