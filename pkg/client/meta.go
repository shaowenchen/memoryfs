package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/cli"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/mountlog"
	"github.com/shaowenchen/memoryfs/pkg/storage"
)

// RemoteMeta implements meta.Backend over HTTP.
type RemoteMeta struct {
	mu        sync.RWMutex
	nodes     []string
	leader    string
	client    *http.Client
	uriPrefix string
}

// NewRemoteMeta creates a remote metadata client.
func NewRemoteMeta(nodes []string) *RemoteMeta {
	r := &RemoteMeta{
		nodes:  append([]string(nil), nodes...),
		client: &http.Client{Timeout: 300 * time.Second},
	}
	if len(nodes) > 0 {
		seed := strings.TrimRight(strings.TrimSpace(nodes[0]), "/")
		r.uriPrefix = cli.NormalizePrefix(cli.DetectPrefix(context.Background(), seed, ""))
		r.nodes = applyNodePrefix(nodes, r.uriPrefix)
	}
	return r
}

func applyNodePrefix(nodes []string, prefix string) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, prefixedNodeURL(n, prefix))
		}
	}
	return out
}

func prefixedNodeURL(base, prefix string) string {
	base = strings.TrimRight(base, "/")
	if prefix == "" || strings.HasSuffix(base, prefix) {
		return base
	}
	return base + prefix
}

type fsReq struct {
	Ino        uint64 `json:"ino,omitempty"`
	ParentIno  uint64 `json:"parent_ino,omitempty"`
	Name       string `json:"name,omitempty"`
	Mode       uint32 `json:"mode,omitempty"`
	UID        uint32 `json:"uid,omitempty"`
	GID        uint32 `json:"gid,omitempty"`
	Target     string `json:"target,omitempty"`
	OldParent  uint64 `json:"old_parent,omitempty"`
	NewParent  uint64 `json:"new_parent,omitempty"`
	OldName    string `json:"old_name,omitempty"`
	NewName    string `json:"new_name,omitempty"`
	ChunkID    string `json:"chunk_id,omitempty"`
	Attr       *meta.Attr `json:"attr,omitempty"`
	Offset     int64      `json:"offset,omitempty"`
	ChunkIdx   int        `json:"chunk_idx,omitempty"`
	BlockIdx   int        `json:"block_idx,omitempty"`
	Data       []byte     `json:"data,omitempty"`
	FileSize   uint64     `json:"file_size,omitempty"`
}

type fsResp struct {
	Attr   *meta.Attr       `json:"attr,omitempty"`
	Attrs  map[string]*meta.Attr `json:"attrs,omitempty"`
	Nodes  []string         `json:"nodes,omitempty"`
	Leader string           `json:"leader,omitempty"`
	Replicas []string       `json:"replicas,omitempty"`
	ChunkID  string         `json:"chunk_id,omitempty"`
	Error  string           `json:"error,omitempty"`
}

func (r *RemoteMeta) GetAttr(ctx context.Context, ino uint64) (*meta.Attr, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/fs/getattr", fsReq{Ino: ino}, &resp, false); err != nil {
		return nil, err
	}
	return resp.Attr, nil
}

func (r *RemoteMeta) Lookup(ctx context.Context, parentIno uint64, name string) (*meta.Attr, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/fs/lookup", fsReq{ParentIno: parentIno, Name: name}, &resp, false); err != nil {
		return nil, err
	}
	return resp.Attr, nil
}

func (r *RemoteMeta) Readdir(ctx context.Context, parentIno uint64) (map[string]*meta.Attr, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/fs/readdir", fsReq{ParentIno: parentIno}, &resp, false); err != nil {
		return nil, err
	}
	return resp.Attrs, nil
}

func (r *RemoteMeta) Mkdir(ctx context.Context, parentIno uint64, name string, mode, uid, gid uint32) (*meta.Attr, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/fs/mkdir", fsReq{ParentIno: parentIno, Name: name, Mode: mode, UID: uid, GID: gid}, &resp, true); err != nil {
		return nil, err
	}
	return resp.Attr, nil
}

func (r *RemoteMeta) Create(ctx context.Context, parentIno uint64, name string, mode, uid, gid uint32) (*meta.Attr, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/fs/create", fsReq{ParentIno: parentIno, Name: name, Mode: mode, UID: uid, GID: gid}, &resp, true); err != nil {
		return nil, err
	}
	return resp.Attr, nil
}

func (r *RemoteMeta) Symlink(ctx context.Context, parentIno uint64, name, target string, uid, gid uint32) (*meta.Attr, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/fs/symlink", fsReq{ParentIno: parentIno, Name: name, Target: target, UID: uid, GID: gid}, &resp, true); err != nil {
		return nil, err
	}
	return resp.Attr, nil
}

func (r *RemoteMeta) Unlink(ctx context.Context, parentIno uint64, name string) (*meta.Attr, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/fs/unlink", fsReq{ParentIno: parentIno, Name: name}, &resp, true); err != nil {
		return nil, err
	}
	return resp.Attr, nil
}

func (r *RemoteMeta) Rmdir(ctx context.Context, parentIno uint64, name string) error {
	var resp fsResp
	return r.post(ctx, "/v1/fs/rmdir", fsReq{ParentIno: parentIno, Name: name}, &resp, true)
}

func (r *RemoteMeta) Rename(ctx context.Context, oldParent, newParent uint64, oldName, newName string) error {
	var resp fsResp
	return r.post(ctx, "/v1/fs/rename", fsReq{OldParent: oldParent, NewParent: newParent, OldName: oldName, NewName: newName}, &resp, true)
}

func (r *RemoteMeta) UpdateAttr(ctx context.Context, attr *meta.Attr) error {
	var resp fsResp
	return r.post(ctx, "/v1/fs/setattr", fsReq{Attr: attr}, &resp, true)
}



func (r *RemoteMeta) ListNodes(ctx context.Context) ([]string, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/cluster/nodes", fsReq{}, &resp, false); err != nil {
		return nil, err
	}
	if len(resp.Nodes) > 0 {
		r.mu.Lock()
		r.nodes = applyNodePrefix(resp.Nodes, r.uriPrefix)
		r.mu.Unlock()
		return r.nodes, nil
	}
	return resp.Nodes, nil
}

// ChunkReplicas returns node URLs that store replicas for a chunk.
func (r *RemoteMeta) ChunkReplicas(ctx context.Context, chunkID string) ([]string, error) {
	var resp fsResp
	if err := r.post(ctx, "/v1/chunks/registry/get", fsReq{ChunkID: chunkID}, &resp, false); err != nil {
		return nil, err
	}
	if len(resp.Replicas) == 0 {
		return nil, fmt.Errorf("no replicas")
	}
	return applyNodePrefix(resp.Replicas, r.uriPrefix), nil
}

func (r *RemoteMeta) ListInos(context.Context) ([]uint64, error) {
	return nil, fmt.Errorf("ListInos not supported on remote meta")
}

func (r *RemoteMeta) PurgeInode(context.Context, uint64) error {
	return fmt.Errorf("PurgeInode not supported on remote meta")
}

func (r *RemoteMeta) Close() error { return nil }

func detachMetaCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return storage.DetachIOContext(parent)
}

func (r *RemoteMeta) post(ctx context.Context, path string, req fsReq, resp *fsResp, write bool) error {
	if write {
		var cancel context.CancelFunc
		ctx, cancel = detachMetaCtx(ctx)
		defer cancel()
	}
	nodes := r.nodeList(write)
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}
	var lastErr error
	for _, base := range nodes {
		url := base + path
		err := r.tryPost(ctx, url, path, req, resp, write)
		if err == nil {
			mountlog.Debugf("meta POST ok %s", url)
			return nil
		}
		if !write && errors.Is(err, meta.ErrNotFound) {
			return err
		}
		lastErr = err
		if shouldWarnMetaErr(err, write) {
			mountlog.Warnf("meta POST %s: %v", url, err)
		}
	}
	return lastErr
}

func (r *RemoteMeta) tryPost(ctx context.Context, url, path string, req fsReq, resp *fsResp, write bool) error {
	if err := r.doPost(ctx, url, req, resp); err != nil {
		if write && resp.Leader != "" {
			leader := prefixedNodeURL(resp.Leader, r.uriPrefix)
			r.SetLeader(leader)
			leaderURL := leader + path
			mountlog.Infof("meta redirect to leader %s", leaderURL)
			if err := r.doPost(ctx, leaderURL, req, resp); err != nil {
				return err
			}
			if resp.Error != "" {
				return mapClientError(resp.Error)
			}
			return nil
		}
		return err
	}
	if resp.Error != "" {
		return mapClientError(resp.Error)
	}
	return nil
}

func shouldWarnMetaErr(err error, _ bool) bool {
	return !errors.Is(err, meta.ErrNotFound)
}

// SetLeader pins mutating metadata requests to the raft leader HTTP URL.
func (r *RemoteMeta) SetLeader(leader string) {
	leader = strings.TrimSpace(leader)
	if leader == "" {
		return
	}
	r.mu.Lock()
	r.leader = prefixedNodeURL(leader, r.uriPrefix)
	r.mu.Unlock()
}

func mapClientError(msg string) error {
	switch msg {
	case "file exists":
		return meta.ErrExists
	case "entry not found":
		return meta.ErrNotFound
	case "directory not empty":
		return meta.ErrNotEmpty
	case "is a directory":
		return meta.ErrIsDir
	case "not a directory":
		return meta.ErrNotDir
	default:
		return fmt.Errorf("%s", msg)
	}
}

func (r *RemoteMeta) doPost(ctx context.Context, url string, req fsReq, resp *fsResp) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	mountlog.Infof("meta doPost req: %s %s", url, string(body))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := r.client.Do(httpReq)
	if err != nil {
		mountlog.Errorf("meta doPost http err: %s %v", url, err)
		return err
	}
	defer func() { _ = httpResp.Body.Close() }()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}
	mountlog.Infof("meta doPost resp: %s status=%d body=%s", url, httpResp.StatusCode, string(raw))
	if err := json.Unmarshal(raw, resp); err != nil {
		return fmt.Errorf("%s: %s", url, string(raw))
	}
	if httpResp.StatusCode == http.StatusTemporaryRedirect || httpResp.StatusCode == http.StatusConflict {
		return fmt.Errorf("redirect")
	}
	if httpResp.StatusCode >= 400 {
		if resp.Error != "" {
			return mapClientError(resp.Error)
		}
		return fmt.Errorf("%s: %s", url, string(raw))
	}
	if resp.Error != "" {
		return mapClientError(resp.Error)
	}
	return nil
}

func (r *RemoteMeta) nodeList(write bool) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if write && r.leader != "" {
		return []string{r.leader}
	}
	out := make([]string, len(r.nodes))
	copy(out, r.nodes)
	return out
}

// Nodes returns configured cluster node HTTP URLs (with URI prefix when used).
func (r *RemoteMeta) Nodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.nodes))
	copy(out, r.nodes)
	return out
}

// RefreshNodes updates node list from cluster.
func (r *RemoteMeta) RefreshNodes(ctx context.Context) error {
	_, err := r.ListNodes(ctx)
	return err
}
