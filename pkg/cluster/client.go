package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// JoinOptions configures follower registration against the bootstrap leader.
type JoinOptions struct {
	LeaderURL   string
	URIPrefix   string
	Member      Member
	MaxAttempts int
	HTTP        *http.Client
}

// Join registers this node with the cluster leader, following 307 redirects until success.
func Join(ctx context.Context, opt JoinOptions) error {
	if opt.MaxAttempts <= 0 {
		opt.MaxAttempts = 60
	}
	if opt.HTTP == nil {
		opt.HTTP = http.DefaultClient
	}
	leader := strings.TrimRight(strings.TrimSpace(opt.LeaderURL), "/")
	var lastErr error
	for attempt := 1; attempt <= opt.MaxAttempts; attempt++ {
		next, nodes, err := joinOnce(ctx, opt.HTTP, leader, opt.URIPrefix, opt.Member)
		if err == nil {
			if len(nodes) > 0 {
				return nil
			}
			return nil
		}
		if next != "" {
			leader = strings.TrimRight(next, "/")
			lastErr = err
			continue
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(min(attempt, 5)) * time.Second):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("join failed")
	}
	return fmt.Errorf("after %d attempts: %w", opt.MaxAttempts, lastErr)
}

func joinOnce(ctx context.Context, client *http.Client, leaderURL, uriPrefix string, member Member) (redirect string, nodes []string, err error) {
	prefix := normalizePrefix(uriPrefix)
	body, _ := json.Marshal(map[string]string{
		"id":        member.ID,
		"raft_addr": member.Raft,
		"http_addr": member.HTTP,
		"grpc_addr": member.GRPC,
		"rdma_addr": member.RDMA,
	})
	joinURL := strings.TrimRight(leaderURL, "/") + prefix + "/v1/cluster/join"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL, bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == http.StatusFound {
		var out struct {
			Leader string   `json:"leader"`
			Nodes  []string `json:"nodes"`
		}
		_ = json.Unmarshal(raw, &out)
		if out.Leader != "" {
			return out.Leader, out.Nodes, fmt.Errorf("join status %d", resp.StatusCode)
		}
		return "", nil, fmt.Errorf("join status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("join status %d", resp.StatusCode)
	}
	var out struct {
		Nodes []string `json:"nodes"`
	}
	_ = json.Unmarshal(raw, &out)
	return "", out.Nodes, nil
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimSuffix(prefix, "/")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
