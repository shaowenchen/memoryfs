package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// NodeOverview summarizes a single cluster node.
type NodeOverview struct {
	URL             string `json:"url"`
	Status          string `json:"status"`
	Role            string `json:"role"`
	NodeState       string `json:"node_state"`
	ClusterEpoch    uint64 `json:"cluster_epoch"`
	RemainingChunks int    `json:"remaining_chunks"`
	Stats           Stats  `json:"stats"`
	Reachable       bool   `json:"reachable"`
	Error           string `json:"error,omitempty"`
}

// ClusterStorage summarizes capacity and usage across reachable nodes.
type ClusterStorage struct {
	TotalDiskQuotaBytes int64 `json:"total_disk_quota_bytes"`
	TotalDiskBytes      int64 `json:"total_disk_bytes"`
	TotalMemBytes       int64 `json:"total_mem_bytes"`
	ReachableNodes      int   `json:"reachable_nodes"`
	TotalNodes          int   `json:"total_nodes"`
}

// ClusterOverview aggregates cluster-wide status for the dashboard.
type ClusterOverview struct {
	NodeID        string         `json:"node_id"`
	Leader        string         `json:"leader"`
	ClusterEpoch  uint64         `json:"cluster_epoch"`
	ReplicaFactor int            `json:"replica_factor"`
	Repair        RepairInfo     `json:"repair"`
	Storage       ClusterStorage `json:"storage"`
	Nodes         []NodeOverview `json:"nodes"`
	GeneratedAt   time.Time      `json:"generated_at"`
}

// ClusterOverview builds a cluster status snapshot from this node.
func (s *Service) ClusterOverview(ctx context.Context) ClusterOverview {
	leader, _ := s.LeaderHTTP()
	status, state, role, epoch, pending := s.Health()
	ov := ClusterOverview{
		NodeID:        s.cfg.NodeID,
		Leader:        leader,
		ClusterEpoch:  epoch,
		ReplicaFactor: s.cfg.ReplicaFactor,
		Repair:        s.RepairInfo(20),
		GeneratedAt:   time.Now(),
		Nodes: []NodeOverview{{
			URL:             s.cfg.NodeHTTP,
			Status:          status,
			Role:            role,
			NodeState:       state,
			ClusterEpoch:    epoch,
			RemainingChunks: pending,
			Stats:           s.Stats(),
			Reachable:       true,
		}},
	}

	nodes, err := s.cfg.Meta.ListNodes(ctx)
	if err != nil {
		return ov
	}
	seen := map[string]struct{}{s.cfg.NodeHTTP: {}}
	for _, url := range nodes {
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		ov.Nodes = append(ov.Nodes, s.fetchNodeOverview(url))
	}
	ov.Storage = summarizeClusterStorage(ov.Nodes)
	return ov
}

func summarizeClusterStorage(nodes []NodeOverview) ClusterStorage {
	var out ClusterStorage
	out.TotalNodes = len(nodes)
	for _, n := range nodes {
		if !n.Reachable {
			continue
		}
		out.ReachableNodes++
		out.TotalDiskBytes += n.Stats.DiskBytes
		out.TotalMemBytes += n.Stats.MemBytes
		out.TotalDiskQuotaBytes += n.Stats.DiskQuotaBytes
	}
	return out
}

func (s *Service) fetchNodeOverview(base string) NodeOverview {
	base = strings.TrimRight(base, "/")
	client := &http.Client{Timeout: 5 * time.Second}
	no := NodeOverview{URL: base}

	resp, err := client.Get(base + "/health")
	if err != nil {
		no.Error = err.Error()
		return no
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var health struct {
		Status          string `json:"status"`
		NodeState       string `json:"node_state"`
		ClusterEpoch    uint64 `json:"cluster_epoch"`
		Role            string `json:"role"`
		RemainingChunks int    `json:"remaining_chunks"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		no.Error = err.Error()
		return no
	}
	no.Reachable = true
	no.Status = health.Status
	no.NodeState = health.NodeState
	no.ClusterEpoch = health.ClusterEpoch
	no.Role = health.Role
	no.RemainingChunks = health.RemainingChunks

	statsResp, err := client.Get(base + "/v1/stats")
	if err == nil {
		defer func() { _ = statsResp.Body.Close() }()
		statsBody, _ := io.ReadAll(statsResp.Body)
		var wrapped struct {
			Stats Stats `json:"stats"`
		}
		if json.Unmarshal(statsBody, &wrapped) == nil {
			no.Stats = wrapped.Stats
		}
	}
	return no
}
