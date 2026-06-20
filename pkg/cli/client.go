package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/service"
)

// Client talks to MemoryFS node HTTP APIs.
type Client struct {
	Seed      string
	URIPrefix string
	Token     string
	HTTP      *http.Client
}

// NewClient creates an HTTP client for a seed node URL.
func NewClient(seed, uriPrefix, token string) *Client {
	return &Client{
		Seed:      strings.TrimRight(strings.TrimSpace(seed), "/"),
		URIPrefix: NormalizePrefix(uriPrefix),
		Token:     token,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
	}
}

// NormalizePrefix ensures a path-style URI prefix.
func NormalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimSuffix(prefix, "/")
}

func (c *Client) path(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return c.Seed + c.URIPrefix + p
}

func (c *Client) do(ctx context.Context, method, p string, body []byte) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.path(p), r)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return raw, resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, resp.StatusCode, nil
}

// DetectPrefix probes common URI prefixes against cluster overview.
func DetectPrefix(ctx context.Context, seed, token string) string {
	for _, p := range []string{"/memoryfs", ""} {
		c := NewClient(seed, p, token)
		if _, err := c.Overview(ctx); err == nil {
			return p
		}
	}
	return ""
}

// Overview fetches aggregated cluster status.
func (c *Client) Overview(ctx context.Context) (*service.ClusterOverview, error) {
	raw, _, err := c.do(ctx, http.MethodGet, "/v1/cluster/overview", nil)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Overview service.ClusterOverview `json:"overview"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, err
	}
	if len(wrapped.Overview.Nodes) == 0 {
		if err := json.Unmarshal(raw, &wrapped.Overview); err != nil {
			return nil, err
		}
	}
	return &wrapped.Overview, nil
}

// PutChunk uploads a chunk by ID.
func (c *Client) PutChunk(ctx context.Context, id string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.path("/chunks/"+id), bytes.NewReader(data))
	if err != nil {
		return err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// GetChunk downloads a chunk by ID.
func (c *Client) GetChunk(ctx context.Context, id string) ([]byte, error) {
	raw, _, err := c.do(ctx, http.MethodGet, "/chunks/"+id, nil)
	return raw, err
}

// DeleteChunk removes a chunk by ID.
func (c *Client) DeleteChunk(ctx context.Context, id string) error {
	_, _, err := c.do(ctx, http.MethodDelete, "/chunks/"+id, nil)
	return err
}
