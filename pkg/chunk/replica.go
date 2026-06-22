package chunk

import (
	"encoding/json"

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

// SelectNodes returns the chain target node URLs (HEAD first, TAIL last) for a chunk.
// Backed by the chain-replication ChainTable: chunkID -> chain -> ordered targets.
func SelectNodes(nodes []string, chunkID string, rf int) ([]string, error) {
	chain, err := ChainFor(nodes, chunkID, rf)
	if err != nil {
		return nil, err
	}
	return chain.NodeURLs(), nil
}
