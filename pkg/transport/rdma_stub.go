//go:build !rdma

package transport

import (
	"context"
	"fmt"
)

// RDMATransport provides high-bandwidth chunk transfer.
// Build with -tags rdma on Linux with rdma-core for hardware RDMA.
type RDMATransport struct {
	fallback ChunkTransport
}

// NewRDMATransport creates an RDMA transport (falls back to gRPC in default build).
func NewRDMATransport(fallback ChunkTransport) *RDMATransport {
	return &RDMATransport{fallback: fallback}
}

func (t *RDMATransport) Kind() Kind { return KindRDMA }

func (t *RDMATransport) PutChunk(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	if t.fallback == nil {
		return fmt.Errorf("rdma not enabled: rebuild with -tags rdma or use grpc/http")
	}
	return t.fallback.PutChunk(ctx, nodeURL, chunkID, data)
}

func (t *RDMATransport) PutChunkReplica(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	if t.fallback == nil {
		return fmt.Errorf("rdma not enabled")
	}
	return t.fallback.PutChunkReplica(ctx, nodeURL, chunkID, data)
}

func (t *RDMATransport) GetChunk(ctx context.Context, nodeURL, chunkID string) ([]byte, error) {
	if t.fallback == nil {
		return nil, fmt.Errorf("rdma not enabled: rebuild with -tags rdma or use grpc/http")
	}
	return t.fallback.GetChunk(ctx, nodeURL, chunkID)
}

func (t *RDMATransport) DeleteChunk(ctx context.Context, nodeURL, chunkID string) error {
	if t.fallback == nil {
		return fmt.Errorf("rdma not enabled")
	}
	return t.fallback.DeleteChunk(ctx, nodeURL, chunkID)
}
