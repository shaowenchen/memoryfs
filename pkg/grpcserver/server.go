package grpcserver

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/shaowenchen/memoryfs/api/memoryfs/v1"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/service"
)

// Server implements the gRPC MemoryFS API.
type Server struct {
	pb.UnimplementedMemoryFSServer
	svc *service.Service
}

// New creates a gRPC server.
func New(svc *service.Service) *Server {
	return &Server{svc: svc}
}

func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	statusStr, state, role, epoch, pending := s.svc.Health()
	return &pb.HealthResponse{
		Status: statusStr, NodeState: state, ClusterEpoch: epoch,
		IsLeader: role == "leader", PendingDrainChunks: int32(pending),
	}, nil
}

func (s *Server) GetLeader(ctx context.Context, _ *pb.GetLeaderRequest) (*pb.GetLeaderResponse, error) {
	leader, err := s.svc.LeaderHTTP()
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%v", err)
	}
	return &pb.GetLeaderResponse{Leader: leader}, nil
}

func (s *Server) ListNodes(ctx context.Context, _ *pb.ListNodesRequest) (*pb.ListNodesResponse, error) {
	nodes, err := s.svc.ListNodes(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.ListNodesResponse{Nodes: nodes}, nil
}

func (s *Server) Join(ctx context.Context, req *pb.JoinRequest) (*pb.JoinResponse, error) {
	if !s.svc.IsLeader() {
		leader, _ := s.svc.LeaderHTTP()
		return nil, status.Errorf(codes.FailedPrecondition, "not leader, try %s", leader)
	}
	if _, err := s.svc.Join(ctx, req.GetId(), req.GetRaftAddr(), req.GetHttpAddr(), req.GetGrpcAddr(), req.GetRdmaAddr()); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.JoinResponse{}, nil
}

func (s *Server) GetAttr(ctx context.Context, req *pb.GetAttrRequest) (*pb.GetAttrResponse, error) {
	attr, err := s.svc.GetAttr(ctx, req.GetIno())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.GetAttrResponse{Attr: toProtoAttr(attr)}, nil
}

func (s *Server) Lookup(ctx context.Context, req *pb.LookupRequest) (*pb.LookupResponse, error) {
	attr, err := s.svc.Lookup(ctx, req.GetParentIno(), req.GetName())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.LookupResponse{Attr: toProtoAttr(attr)}, nil
}

func (s *Server) Readdir(ctx context.Context, req *pb.ReaddirRequest) (*pb.ReaddirResponse, error) {
	entries, err := s.svc.Readdir(ctx, req.GetParentIno())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	out := make(map[string]*pb.Attr, len(entries))
	for k, v := range entries {
		out[k] = toProtoAttr(v)
	}
	return &pb.ReaddirResponse{Entries: out}, nil
}

func (s *Server) Mkdir(ctx context.Context, req *pb.MkdirRequest) (*pb.MkdirResponse, error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	attr, err := s.svc.Mkdir(ctx, req.GetParentIno(), req.GetName(), req.GetMode(), req.GetUid(), req.GetGid())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.MkdirResponse{Attr: toProtoAttr(attr)}, nil
}

func (s *Server) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	attr, err := s.svc.Create(ctx, req.GetParentIno(), req.GetName(), req.GetMode(), req.GetUid(), req.GetGid())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.CreateResponse{Attr: toProtoAttr(attr)}, nil
}

func (s *Server) Symlink(ctx context.Context, req *pb.SymlinkRequest) (*pb.SymlinkResponse, error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	attr, err := s.svc.Symlink(ctx, req.GetParentIno(), req.GetName(), req.GetTarget(), req.GetUid(), req.GetGid())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.SymlinkResponse{Attr: toProtoAttr(attr)}, nil
}

func (s *Server) Unlink(ctx context.Context, req *pb.UnlinkRequest) (*pb.UnlinkResponse, error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	attr, err := s.svc.Unlink(ctx, req.GetParentIno(), req.GetName())
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.UnlinkResponse{Attr: toProtoAttr(attr)}, nil
}

func (s *Server) Rmdir(ctx context.Context, req *pb.RmdirRequest) (*pb.RmdirResponse, error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	if err := s.svc.Rmdir(ctx, req.GetParentIno(), req.GetName()); err != nil {
		return nil, toStatus(err)
	}
	return &pb.RmdirResponse{}, nil
}

func (s *Server) Rename(ctx context.Context, req *pb.RenameRequest) (*pb.RenameResponse, error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	if err := s.svc.Rename(ctx, req.GetOldParent(), req.GetNewParent(), req.GetOldName(), req.GetNewName()); err != nil {
		return nil, toStatus(err)
	}
	return &pb.RenameResponse{}, nil
}

func (s *Server) SetAttr(ctx context.Context, req *pb.SetAttrRequest) (*pb.SetAttrResponse, error) {
	if err := s.requireLeader(); err != nil {
		return nil, err
	}
	attr := fromProtoAttr(req.GetAttr())
	if err := s.svc.SetAttr(ctx, attr); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.SetAttrResponse{Attr: toProtoAttr(attr)}, nil
}

func (s *Server) PutChunk(stream pb.MemoryFS_PutChunkServer) error {
	var chunkID string
	var data []byte
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if chunkID == "" {
			chunkID = msg.GetChunkId()
		}
		data = append(data, msg.GetData()...)
	}
	replicas, err := s.svc.PutChunk(stream.Context(), chunkID, data)
	if err != nil {
		return status.Errorf(codes.Internal, "%v", err)
	}
	return stream.SendAndClose(&pb.PutChunkResponse{Replicas: replicas})
}

func (s *Server) GetChunk(req *pb.GetChunkRequest, stream pb.MemoryFS_GetChunkServer) error {
	data, err := s.svc.GetChunk(stream.Context(), req.GetChunkId())
	if err != nil {
		return status.Errorf(codes.NotFound, "%v", err)
	}
	return stream.Send(&pb.GetChunkResponse{Data: data})
}

func (s *Server) DeleteChunk(ctx context.Context, req *pb.DeleteChunkRequest) (*pb.DeleteChunkResponse, error) {
	if err := s.svc.DeleteChunk(ctx, req.GetChunkId()); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.DeleteChunkResponse{}, nil
}

func (s *Server) Drain(ctx context.Context, req *pb.DrainRequest) (*pb.DrainResponse, error) {
	remaining, drained, err := s.svc.Drain(ctx, req.GetForce())
	if err != nil {
		return &pb.DrainResponse{RemainingChunks: int32(remaining), Drained: drained}, status.Errorf(codes.Aborted, "%v", err)
	}
	return &pb.DrainResponse{RemainingChunks: 0, Drained: true}, nil
}

func (s *Server) Ready(ctx context.Context, _ *pb.ReadyRequest) (*pb.ReadyResponse, error) {
	s.svc.Ready(ctx)
	return &pb.ReadyResponse{Status: "active"}, nil
}

func (s *Server) requireLeader() error {
	if !s.svc.IsLeader() {
		leader, _ := s.svc.LeaderHTTP()
		return status.Errorf(codes.FailedPrecondition, "not leader, try %s", leader)
	}
	return nil
}

func toProtoAttr(a *meta.Attr) *pb.Attr {
	if a == nil {
		return nil
	}
	return &pb.Attr{
		Ino: a.Ino, Mode: a.Mode, Size: a.Size, Uid: a.UID, Gid: a.GID,
		Mtime: a.Mtime, Nlink: a.Nlink, Target: a.Target, Chunks: a.Chunks,
	}
}

func fromProtoAttr(a *pb.Attr) *meta.Attr {
	if a == nil {
		return nil
	}
	return &meta.Attr{
		Ino: a.GetIno(), Mode: a.GetMode(), Size: a.GetSize(), UID: a.GetUid(), GID: a.GetGid(),
		Mtime: a.GetMtime(), Nlink: a.GetNlink(), Target: a.GetTarget(), Chunks: a.GetChunks(),
	}
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, meta.ErrNotFound):
		return status.Error(codes.NotFound, meta.MapError(err))
	case errors.Is(err, meta.ErrExists):
		return status.Error(codes.AlreadyExists, meta.MapError(err))
	case errors.Is(err, meta.ErrNotEmpty):
		return status.Error(codes.FailedPrecondition, meta.MapError(err))
	default:
		return status.Errorf(codes.Internal, "%v", err)
	}
}
