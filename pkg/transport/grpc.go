package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/shaowenchen/memoryfs/pkg/ports"
	pb "github.com/shaowenchen/memoryfs/api/memoryfs/v1"
)

// GRPCTransport uses gRPC streaming chunk APIs.
type GRPCTransport struct {
	dial func(target string) (*grpc.ClientConn, error)
}

// NewGRPCTransport creates a gRPC chunk transport.
func NewGRPCTransport() *GRPCTransport {
	return NewGRPCTransportWithHTTP(nil)
}

// NewGRPCTransportWithHTTP keeps compatibility with older callsites.
func NewGRPCTransportWithHTTP(_ *HTTPTransport) *GRPCTransport {
	return &GRPCTransport{
		dial: func(target string) (*grpc.ClientConn, error) {
			return grpc.NewClient(target,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
					d := net.Dialer{Timeout: DialTimeout}
					return d.DialContext(ctx, "tcp", addr)
				}),
				grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16<<20), grpc.MaxCallSendMsgSize(16<<20)),
			)
		},
	}
}

func (t *GRPCTransport) Kind() Kind { return KindGRPC }

func (t *GRPCTransport) PutChunk(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	return t.PutChunkWithOptions(ctx, nodeURL, chunkID, data, ChunkWriteOptions{})
}

func (t *GRPCTransport) PutChunkReplica(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	return t.PutChunkWithOptions(ctx, nodeURL, chunkID, data, ChunkWriteOptions{Replica: true})
}

func (t *GRPCTransport) PutChunkWithOptions(ctx context.Context, nodeURL, chunkID string, data []byte, opts ChunkWriteOptions) error {
	return t.putChunk(ctx, nodeURL, chunkID, data, opts)
}

func (t *GRPCTransport) putChunk(ctx context.Context, nodeURL, chunkID string, data []byte, opts ChunkWriteOptions) error {
	conn, err := t.dial(normalizeGRPC(nodeURL))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	client := pb.NewMemoryFSClient(conn)
	stream, err := client.PutChunk(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&pb.PutChunkRequest{
		ChunkId:    chunkID,
		Data:       data,
		Replica:    opts.Replica,
		Stage:      opts.Stage,
		ChainId:    opts.ChainID,
		ChainVer:   opts.ChainVer,
		UpdateVer:  opts.UpdateVer,
		CommitVer:  opts.CommitVer,
		Replicas:   opts.Replicas,
		FromClient: opts.FromClient,
		Syncing:    opts.Syncing,
	}); err != nil {
		return err
	}
	_, err = stream.CloseAndRecv()
	return err
}

func (t *GRPCTransport) GetChunk(ctx context.Context, nodeURL, chunkID string) ([]byte, error) {
	return t.GetChunkWithOptions(ctx, nodeURL, chunkID, ChunkReadOptions{})
}

func (t *GRPCTransport) GetChunkWithOptions(ctx context.Context, nodeURL, chunkID string, opts ChunkReadOptions) ([]byte, error) {
	conn, err := t.dial(normalizeGRPC(nodeURL))
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	client := pb.NewMemoryFSClient(conn)
	stream, err := client.GetChunk(ctx, &pb.GetChunkRequest{
		ChunkId:          chunkID,
		AllowUncommitted: opts.AllowUncommitted,
	})
	if err != nil {
		return nil, err
	}
	var out []byte
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, msg.GetData()...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("chunk not found")
	}
	return out, nil
}

func (t *GRPCTransport) DeleteChunk(ctx context.Context, nodeURL, chunkID string) error {
	conn, err := t.dial(normalizeGRPC(nodeURL))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	client := pb.NewMemoryFSClient(conn)
	_, err = client.DeleteChunk(ctx, &pb.DeleteChunkRequest{ChunkId: chunkID})
	return err
}

func normalizeGRPC(nodeURL string) string {
	addr := strings.TrimPrefix(strings.TrimPrefix(nodeURL, "http://"), "https://")
	if idx := strings.Index(addr, "/"); idx >= 0 {
		addr = addr[:idx]
	}
	if !strings.Contains(addr, ":") {
		return addr + ":" + ports.GRPC
	}
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		return addr
	}
	if port == ports.HTTP {
		return host + ":" + ports.GRPC
	}
	return addr
}

// DialTimeout for grpc connections.
const DialTimeout = 10 * time.Second
