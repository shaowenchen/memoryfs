package transport

import "context"

// Kind identifies the transport protocol.
type Kind string

const (
	KindHTTP Kind = "http"
	KindGRPC Kind = "grpc"
	KindRDMA Kind = "rdma"
)

// ChunkWriteOptions carries CRAQ write metadata between chain targets.
type ChunkWriteOptions struct {
	Replica   bool
	FromClient bool
	Stage     string
	ChainID   uint32
	ChainVer  uint64
	UpdateVer uint64
	CommitVer uint64
	Replicas  []string
	Syncing   bool
}

// ChunkReadOptions controls read visibility.
type ChunkReadOptions struct {
	AllowUncommitted bool
}

// ChunkTransport moves chunk data between nodes and clients.
type ChunkTransport interface {
	Kind() Kind
	PutChunk(ctx context.Context, nodeURL, chunkID string, data []byte) error
	PutChunkReplica(ctx context.Context, nodeURL, chunkID string, data []byte) error
	PutChunkWithOptions(ctx context.Context, nodeURL, chunkID string, data []byte, opts ChunkWriteOptions) error
	GetChunk(ctx context.Context, nodeURL, chunkID string) ([]byte, error)
	GetChunkWithOptions(ctx context.Context, nodeURL, chunkID string, opts ChunkReadOptions) ([]byte, error)
	DeleteChunk(ctx context.Context, nodeURL, chunkID string) error
}

// MultiTransport tries transports in order (e.g. RDMA then gRPC then HTTP).
type MultiTransport struct {
	transports []ChunkTransport
}

// Kind returns the composite transport kind label.
func (m *MultiTransport) Kind() Kind { return KindGRPC }

// NewMultiTransport creates a fallback transport chain.
func NewMultiTransport(transports ...ChunkTransport) *MultiTransport {
	return &MultiTransport{transports: transports}
}

func (m *MultiTransport) PutChunk(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	var last error
	for _, t := range m.transports {
		if err := t.PutChunk(ctx, nodeURL, chunkID, data); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return last
}

func (m *MultiTransport) PutChunkReplica(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	var last error
	for _, t := range m.transports {
		if err := t.PutChunkReplica(ctx, nodeURL, chunkID, data); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return last
}

func (m *MultiTransport) PutChunkWithOptions(ctx context.Context, nodeURL, chunkID string, data []byte, opts ChunkWriteOptions) error {
	var last error
	for _, t := range m.transports {
		if err := t.PutChunkWithOptions(ctx, nodeURL, chunkID, data, opts); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return last
}

func (m *MultiTransport) GetChunk(ctx context.Context, nodeURL, chunkID string) ([]byte, error) {
	return m.GetChunkWithOptions(ctx, nodeURL, chunkID, ChunkReadOptions{})
}

func (m *MultiTransport) GetChunkWithOptions(ctx context.Context, nodeURL, chunkID string, opts ChunkReadOptions) ([]byte, error) {
	var last error
	for _, t := range m.transports {
		data, err := t.GetChunkWithOptions(ctx, nodeURL, chunkID, opts)
		if err == nil {
			return data, nil
		}
		last = err
	}
	return nil, last
}

func (m *MultiTransport) DeleteChunk(ctx context.Context, nodeURL, chunkID string) error {
	var last error
	for _, t := range m.transports {
		if err := t.DeleteChunk(ctx, nodeURL, chunkID); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return last
}
