package node

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/service"
)

// Server is the unified MemoryFS node HTTP server (API + dashboard, single binary).
type Server struct {
	svc       *service.Service
	mux       *http.ServeMux
	apiToken  string
	uriPrefix string
}

// NewServer creates a node HTTP server.
func NewServer(svc *service.Service, apiToken, uriPrefix string) *Server {
	s := &Server{
		svc:       svc,
		mux:       http.NewServeMux(),
		apiToken:  apiToken,
		uriPrefix: NormalizeURIPrefix(uriPrefix),
	}
	s.routes()
	return s
}

// URIPrefix returns the configured HTTP path prefix for dashboard and API routes.
func (s *Server) URIPrefix() string { return s.uriPrefix }

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.handleDashboard)
	s.mux.HandleFunc("/dashboard", s.handleDashboard)
	s.mux.HandleFunc("/dashboard/", s.handleDashboard)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/v1/cluster/overview", s.handleOverview)
	s.mux.HandleFunc("/v1/repair", s.handleRepairStatus)
	s.mux.HandleFunc("/v1/repair/run", s.handleRepair)
	s.mux.HandleFunc("/v1/cluster/leader", s.handleLeader)
	s.mux.HandleFunc("/v1/cluster/nodes", s.handleNodes)
	s.mux.HandleFunc("/v1/cluster/join", s.handleJoin)
	s.mux.HandleFunc("/v1/cluster/remove", s.handleRemove)
	s.mux.HandleFunc("/v1/cluster/leave", s.handleLeave)
	s.mux.HandleFunc("/v1/lifecycle/drain", s.handleDrain)
	s.mux.HandleFunc("/v1/lifecycle/ready", s.handleReady)
	s.mux.HandleFunc("/v1/stats", s.handleStats)
	s.mux.HandleFunc("/v1/gc", s.handleGC)
	s.mux.HandleFunc("/v1/chunks/registry/set", s.handleWrite(s.registrySet))
	s.mux.HandleFunc("/v1/chunks/registry/delete", s.handleWrite(s.registryDelete))
	s.mux.HandleFunc("/v1/chunks/registry/get", s.handleFS(s.registryGet))
	s.mux.HandleFunc("/v1/fs/getattr", s.handleFS(s.getattr))
	s.mux.HandleFunc("/v1/fs/lookup", s.handleFS(s.lookup))
	s.mux.HandleFunc("/v1/fs/readdir", s.handleFS(s.readdir))
	s.mux.HandleFunc("/v1/fs/mkdir", s.handleWrite(s.mkdir))
	s.mux.HandleFunc("/v1/fs/create", s.handleWrite(s.create))
	s.mux.HandleFunc("/v1/fs/symlink", s.handleWrite(s.symlink))
	s.mux.HandleFunc("/v1/fs/unlink", s.handleWrite(s.unlink))
	s.mux.HandleFunc("/v1/fs/rmdir", s.handleWrite(s.rmdir))
	s.mux.HandleFunc("/v1/fs/rename", s.handleWrite(s.rename))
	s.mux.HandleFunc("/v1/fs/setattr", s.handleWrite(s.setattr))
	s.mux.HandleFunc("/v1/fs/write", s.handleWrite(s.writeBlock))
	s.mux.HandleFunc("/chunks/", s.handleChunks)
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	h := http.Handler(s.mux)
	h = PrefixMiddleware(s.uriPrefix, h)
	return AuthMiddleware(s.apiToken, h)
}

// Service returns the underlying service.
func (s *Server) Service() *service.Service { return s.svc }

type fsRequest struct {
	Ino       uint64     `json:"ino,omitempty"`
	ParentIno uint64     `json:"parent_ino,omitempty"`
	Name      string     `json:"name,omitempty"`
	Mode      uint32     `json:"mode,omitempty"`
	UID       uint32     `json:"uid,omitempty"`
	GID       uint32     `json:"gid,omitempty"`
	Target    string     `json:"target,omitempty"`
	OldParent uint64     `json:"old_parent,omitempty"`
	NewParent uint64     `json:"new_parent,omitempty"`
	OldName   string     `json:"old_name,omitempty"`
	NewName   string     `json:"new_name,omitempty"`
	Attr      *meta.Attr `json:"attr,omitempty"`
	Force     bool       `json:"force,omitempty"`
	ChunkID   string     `json:"chunk_id,omitempty"`
	Offset    int64      `json:"offset,omitempty"`
	ChunkIdx  int        `json:"chunk_idx,omitempty"`
	BlockIdx  int        `json:"block_idx,omitempty"`
	Data      []byte     `json:"data,omitempty"`
	FileSize  uint64     `json:"file_size,omitempty"`
	Replicas  []string   `json:"replicas,omitempty"`
	Epoch     uint64     `json:"epoch,omitempty"`
}

type fsResponse struct {
	Attr             *meta.Attr            `json:"attr,omitempty"`
	Attrs            map[string]*meta.Attr `json:"attrs,omitempty"`
	Nodes            []string              `json:"nodes,omitempty"`
	Leader           string                `json:"leader,omitempty"`
	Error            string                `json:"error,omitempty"`
	Status           string                `json:"status,omitempty"`
	NodeState        string                `json:"node_state,omitempty"`
	ClusterEpoch     uint64                `json:"cluster_epoch,omitempty"`
	Role             string                `json:"role,omitempty"`
	Drained          bool                  `json:"drained,omitempty"`
	RemainingChunks  int                   `json:"remaining_chunks,omitempty"`
	Stats            *service.Stats        `json:"stats,omitempty"`
	GCRemoved        int                   `json:"gc_removed,omitempty"`
	Overview         *service.ClusterOverview `json:"overview,omitempty"`
	Repair           *service.RepairInfo   `json:"repair,omitempty"`
	RepairFixed      int                   `json:"repair_fixed,omitempty"`
	RepairFailed     int                   `json:"repair_failed,omitempty"`
	RepairPending    int                   `json:"repair_pending,omitempty"`
	Replicas         []string              `json:"replicas,omitempty"`
	ChunkID          string                `json:"chunk_id,omitempty"`
}

type joinRequest struct {
	ID       string `json:"id"`
	RaftAddr string `json:"raft_addr"`
	HTTPAddr string `json:"http_addr"`
	GRPCAddr string `json:"grpc_addr"`
	RDMAAddr string `json:"rdma_addr"`
}

type removeRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.svc.UpdateMetrics()
	promhttp.Handler().ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	status, state, role, epoch, pending := s.svc.Health()
	writeJSON(w, http.StatusOK, fsResponse{
		Status: status, NodeState: state, ClusterEpoch: epoch, Role: role,
		RemainingChunks: pending,
	})
}

func (s *Server) handleLeader(w http.ResponseWriter, _ *http.Request) {
	leader, err := s.svc.LeaderHTTP()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, fsResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, fsResponse{Leader: leader})
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.svc.ListNodes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, fsResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, fsResponse{Nodes: nodes})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.svc.IsLeader() {
		leader, _ := s.svc.LeaderHTTP()
		writeJSON(w, http.StatusTemporaryRedirect, fsResponse{Leader: leader})
		return
	}
	var req joinRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, fsResponse{Error: err.Error()})
		return
	}
	nodes, err := s.svc.Join(r.Context(), req.ID, req.RaftAddr, req.HTTPAddr, req.GRPCAddr, req.RDMAAddr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, fsResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, fsResponse{Nodes: nodes})
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.svc.IsLeader() {
		leader, _ := s.svc.LeaderHTTP()
		writeJSON(w, http.StatusTemporaryRedirect, fsResponse{Leader: leader})
		return
	}
	var req removeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, fsResponse{Error: err.Error()})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, fsResponse{Error: "missing id"})
		return
	}
	if err := s.svc.RemoveNode(r.Context(), req.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, fsResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, fsResponse{})
}

func (s *Server) handleLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	remaining, drained, err := s.svc.Leave(r.Context())
	if err != nil {
		writeJSON(w, http.StatusConflict, fsResponse{Error: err.Error(), RemainingChunks: remaining, Drained: drained})
		return
	}
	writeJSON(w, http.StatusOK, fsResponse{Drained: drained, RemainingChunks: remaining})
}

func (s *Server) handleDrain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fsRequest
	_ = decodeJSON(r, &req)
	remaining, drained, err := s.svc.Drain(r.Context(), req.Force)
	if err != nil {
		writeJSON(w, http.StatusConflict, fsResponse{Error: err.Error(), RemainingChunks: remaining, Drained: drained})
		return
	}
	writeJSON(w, http.StatusOK, fsResponse{Drained: drained, RemainingChunks: remaining})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.svc.Ready(r.Context())
	writeJSON(w, http.StatusOK, fsResponse{Status: "active"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := s.svc.Stats()
	writeJSON(w, http.StatusOK, fsResponse{Stats: &st})
}

func (s *Server) handleGC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := s.svc.RunGC(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, fsResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, fsResponse{GCRemoved: n})
}

type fsHandler func(context.Context, fsRequest) (fsResponse, int)

func (s *Server) handleFS(fn fsHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req fsRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, fsResponse{Error: err.Error()})
			return
		}
		resp, code := fn(r.Context(), req)
		writeJSON(w, code, resp)
	}
}

func (s *Server) handleWrite(fn fsHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.svc.IsLeader() {
			leader, _ := s.svc.LeaderHTTP()
			writeJSON(w, http.StatusTemporaryRedirect, fsResponse{Leader: leader})
			return
		}
		var req fsRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, fsResponse{Error: err.Error()})
			return
		}
		resp, code := fn(r.Context(), req)
		writeJSON(w, code, resp)
	}
}

func (s *Server) getattr(ctx context.Context, req fsRequest) (fsResponse, int) {
	attr, err := s.svc.GetAttr(ctx, req.Ino)
	if err != nil {
		return fsResponse{Error: meta.MapError(err)}, http.StatusNotFound
	}
	return fsResponse{Attr: attr}, http.StatusOK
}

func (s *Server) lookup(ctx context.Context, req fsRequest) (fsResponse, int) {
	attr, err := s.svc.Lookup(ctx, req.ParentIno, req.Name)
	if err != nil {
		return fsResponse{Error: meta.MapError(err)}, http.StatusNotFound
	}
	return fsResponse{Attr: attr}, http.StatusOK
}

func (s *Server) readdir(ctx context.Context, req fsRequest) (fsResponse, int) {
	attrs, err := s.svc.Readdir(ctx, req.ParentIno)
	if err != nil {
		return fsResponse{Error: err.Error()}, http.StatusInternalServerError
	}
	return fsResponse{Attrs: attrs}, http.StatusOK
}

func (s *Server) mkdir(ctx context.Context, req fsRequest) (fsResponse, int) {
	attr, err := s.svc.Mkdir(ctx, req.ParentIno, req.Name, req.Mode, req.UID, req.GID)
	if err != nil {
		return fsResponse{Error: meta.MapError(err)}, http.StatusConflict
	}
	return fsResponse{Attr: attr}, http.StatusOK
}

func (s *Server) create(ctx context.Context, req fsRequest) (fsResponse, int) {
	attr, err := s.svc.Create(ctx, req.ParentIno, req.Name, req.Mode, req.UID, req.GID)
	if err != nil {
		return fsResponse{Error: meta.MapError(err)}, http.StatusConflict
	}
	return fsResponse{Attr: attr}, http.StatusOK
}

func (s *Server) symlink(ctx context.Context, req fsRequest) (fsResponse, int) {
	attr, err := s.svc.Symlink(ctx, req.ParentIno, req.Name, req.Target, req.UID, req.GID)
	if err != nil {
		return fsResponse{Error: meta.MapError(err)}, http.StatusConflict
	}
	return fsResponse{Attr: attr}, http.StatusOK
}

func (s *Server) unlink(ctx context.Context, req fsRequest) (fsResponse, int) {
	attr, err := s.svc.Unlink(ctx, req.ParentIno, req.Name)
	if err != nil {
		return fsResponse{Error: meta.MapError(err)}, http.StatusNotFound
	}
	return fsResponse{Attr: attr}, http.StatusOK
}

func (s *Server) rmdir(ctx context.Context, req fsRequest) (fsResponse, int) {
	if err := s.svc.Rmdir(ctx, req.ParentIno, req.Name); err != nil {
		return fsResponse{Error: meta.MapError(err)}, http.StatusConflict
	}
	return fsResponse{}, http.StatusOK
}

func (s *Server) rename(ctx context.Context, req fsRequest) (fsResponse, int) {
	if err := s.svc.Rename(ctx, req.OldParent, req.NewParent, req.OldName, req.NewName); err != nil {
		return fsResponse{Error: meta.MapError(err)}, http.StatusConflict
	}
	return fsResponse{}, http.StatusOK
}

func (s *Server) setattr(ctx context.Context, req fsRequest) (fsResponse, int) {
	if req.Attr == nil {
		return fsResponse{Error: "missing attr"}, http.StatusBadRequest
	}
	if err := s.svc.SetAttr(ctx, req.Attr); err != nil {
		return fsResponse{Error: err.Error()}, http.StatusInternalServerError
	}
	return fsResponse{Attr: req.Attr}, http.StatusOK
}

func (s *Server) writeBlock(ctx context.Context, req fsRequest) (fsResponse, int) {
	if req.Ino == 0 {
		return fsResponse{Error: "missing ino"}, http.StatusBadRequest
	}
	if len(req.Data) == 0 {
		return fsResponse{Error: "missing data"}, http.StatusBadRequest
	}
	var attr *meta.Attr
	var err error
	if req.FileSize > 0 || req.ChunkIdx > 0 || req.BlockIdx > 0 {
		attr, err = s.svc.WriteBlock(ctx, req.Ino, req.ChunkIdx, req.BlockIdx, req.Data, req.FileSize)
	} else {
		attr, err = s.svc.WriteAt(ctx, req.Ino, req.Offset, req.Data)
	}
	if err != nil {
		return fsResponse{Error: err.Error()}, http.StatusInternalServerError
	}
	return fsResponse{Attr: attr}, http.StatusOK
}

func (s *Server) registrySet(ctx context.Context, req fsRequest) (fsResponse, int) {
	if req.ChunkID == "" {
		return fsResponse{Error: "missing chunk_id"}, http.StatusBadRequest
	}
	if err := s.svc.ApplyRegistrySet(ctx, req.ChunkID, req.Replicas, req.Epoch); err != nil {
		return fsResponse{Error: err.Error()}, http.StatusInternalServerError
	}
	return fsResponse{}, http.StatusOK
}

func (s *Server) registryGet(ctx context.Context, req fsRequest) (fsResponse, int) {
	if req.ChunkID == "" {
		return fsResponse{Error: "missing chunk_id"}, http.StatusBadRequest
	}
	loc, err := s.svc.GetChunkRegistry(ctx, req.ChunkID)
	if err != nil {
		return fsResponse{Error: "not found"}, http.StatusNotFound
	}
	return fsResponse{ChunkID: req.ChunkID, Replicas: loc.Replicas}, http.StatusOK
}

func (s *Server) registryDelete(ctx context.Context, req fsRequest) (fsResponse, int) {
	if req.ChunkID == "" {
		return fsResponse{Error: "missing chunk_id"}, http.StatusBadRequest
	}
	if err := s.svc.ApplyRegistryDelete(ctx, req.ChunkID); err != nil {
		return fsResponse{Error: err.Error()}, http.StatusInternalServerError
	}
	return fsResponse{}, http.StatusOK
}

func (s *Server) handleChunks(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/chunks/")
	if id == "" {
		http.Error(w, "missing chunk id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		data, err := s.svc.GetChunk(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("replica") == "1" {
			if err := s.svc.StoreChunkLocal(id, data); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if _, err := s.svc.PutChunk(r.Context(), id, data); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		_ = s.svc.DeleteChunk(r.Context(), id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, code int, resp fsResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("write json: %v", err)
	}
}
