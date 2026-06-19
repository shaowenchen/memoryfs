package chunk

import (
	"context"
	"fmt"
)

// Rebuild pulls missing local chunks from peer replicas.
func Rebuild(ctx context.Context, nodeHTTP string, rf int, store Store, registry *Registry, nodes []string, fetch func(ctx context.Context, nodeURL, chunkID string) ([]byte, error)) (int, error) {
	if registry == nil || fetch == nil {
		return 0, nil
	}
	indexed, err := registry.ListIndexed()
	if err != nil {
		return 0, err
	}
	local := make(map[string]struct{}, len(store.List()))
	for _, id := range store.List() {
		local[id] = struct{}{}
	}

	rebuilt := 0
	for _, chunkID := range indexed {
		if _, ok := local[chunkID]; ok {
			continue
		}
		loc, err := registry.Get(chunkID)
		if err != nil {
			// fallback: derive replica set from hash
			replicas, err := SelectNodes(nodes, chunkID, rf)
			if err != nil {
				continue
			}
			loc = &Location{ChunkID: chunkID, Replicas: replicas}
		}
		if !contains(loc.Replicas, nodeHTTP) {
			continue
		}
		if err := pullFromPeers(ctx, chunkID, nodeHTTP, loc.Replicas, store, fetch); err != nil {
			return rebuilt, fmt.Errorf("rebuild %s: %w", chunkID, err)
		}
		rebuilt++
	}
	return rebuilt, nil
}

func pullFromPeers(ctx context.Context, chunkID, self string, replicas []string, store Store, fetch func(ctx context.Context, nodeURL, chunkID string) ([]byte, error)) error {
	for _, node := range replicas {
		if node == self {
			continue
		}
		data, err := fetch(ctx, node, chunkID)
		if err != nil {
			continue
		}
		return store.Put(chunkID, data)
	}
	return fmt.Errorf("no peer replica available for %s", chunkID)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// NodeInReplicas reports whether nodeHTTP should hold chunkID.
func NodeInReplicas(nodes []string, chunkID, nodeHTTP string, rf int) (bool, []string, error) {
	replicas, err := SelectNodes(nodes, chunkID, rf)
	if err != nil {
		return false, nil, err
	}
	return contains(replicas, nodeHTTP), replicas, nil
}
