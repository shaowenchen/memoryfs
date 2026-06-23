package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/mountlog"
)

// HTTPTransport uses REST chunk endpoints.
type HTTPTransport struct {
	client *http.Client
	prefix string
}

// NewHTTPTransport creates an HTTP chunk transport.
func NewHTTPTransport() *HTTPTransport {
	return NewPrefixedHTTPTransport("")
}

// NewPrefixedHTTPTransport creates an HTTP chunk transport with a URI prefix (e.g. /memoryfs).
// Uses a short dial timeout so unreachable peers fail fast instead of blocking the
// FUSE write path on the OS TCP SYN timeout (~75 s on Linux).
func NewPrefixedHTTPTransport(prefix string) *HTTPTransport {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       60 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &HTTPTransport{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		prefix: normalizeURIPrefix(prefix),
	}
}

func normalizeURIPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimSuffix(prefix, "/")
}

func (t *HTTPTransport) chunkURL(nodeURL, chunkID string, replica bool) string {
	url := strings.TrimRight(nodeURL, "/") + t.prefix + "/chunks/" + chunkID
	if replica {
		url += "?replica=1"
	}
	return url
}

func (t *HTTPTransport) Kind() Kind { return KindHTTP }

func (t *HTTPTransport) PutChunk(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	return t.PutChunkWithOptions(ctx, nodeURL, chunkID, data, ChunkWriteOptions{})
}

func (t *HTTPTransport) PutChunkReplica(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	return t.PutChunkWithOptions(ctx, nodeURL, chunkID, data, ChunkWriteOptions{Replica: true})
}

func (t *HTTPTransport) PutChunkWithOptions(ctx context.Context, nodeURL, chunkID string, data []byte, opts ChunkWriteOptions) error {
	chunkURL := t.chunkURL(nodeURL, chunkID, opts.Replica)
	values := url.Values{}
	if opts.Stage != "" {
		values.Set("stage", opts.Stage)
	}
	if opts.ChainID != 0 {
		values.Set("chain_id", strconv.FormatUint(uint64(opts.ChainID), 10))
	}
	if opts.ChainVer != 0 {
		values.Set("chain_ver", strconv.FormatUint(opts.ChainVer, 10))
	}
	if opts.UpdateVer != 0 {
		values.Set("update_ver", strconv.FormatUint(opts.UpdateVer, 10))
	}
	if opts.CommitVer != 0 {
		values.Set("commit_ver", strconv.FormatUint(opts.CommitVer, 10))
	}
	if len(opts.Replicas) > 0 {
		values.Set("replicas", strings.Join(opts.Replicas, ","))
	}
	if opts.FromClient {
		values.Set("from_client", "1")
	}
	if opts.Syncing {
		values.Set("syncing", "1")
	}
	if encoded := values.Encode(); encoded != "" {
		if strings.Contains(chunkURL, "?") {
			chunkURL += "&" + encoded
		} else {
			chunkURL += "?" + encoded
		}
	}
	mountlog.Infof("chunk PUT start url=%s bytes=%d replica=%v stage=%s", chunkURL, len(data), opts.Replica, opts.Stage)
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, chunkURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		mountlog.Errorf("chunk PUT url=%s bytes=%d err: %v", chunkURL, len(data), err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	mountlog.Infof("chunk PUT done url=%s status=%d duration=%v body=%s", chunkURL, resp.StatusCode, time.Since(start), string(raw))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("http put chunk status=%d body=%s", resp.StatusCode, raw)
	}
	return nil
}

func (t *HTTPTransport) GetChunk(ctx context.Context, nodeURL, chunkID string) ([]byte, error) {
	return t.GetChunkWithOptions(ctx, nodeURL, chunkID, ChunkReadOptions{})
}

func (t *HTTPTransport) GetChunkWithOptions(ctx context.Context, nodeURL, chunkID string, opts ChunkReadOptions) ([]byte, error) {
	chunkURL := t.chunkURL(nodeURL, chunkID, false)
	if opts.AllowUncommitted {
		chunkURL += "?allow_uncommitted=1"
	}
	mountlog.Debugf("chunk GET start url=%s", chunkURL)
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chunkURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		mountlog.Errorf("chunk GET url=%s: %v", chunkURL, err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		mountlog.Debugf("chunk GET miss url=%s in %s", chunkURL, time.Since(start))
		return nil, fmt.Errorf("chunk not found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("http get chunk status=%d body=%s", resp.StatusCode, body)
		mountlog.Errorf("chunk GET url=%s: %v", chunkURL, err)
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		mountlog.Errorf("chunk GET read body url=%s: %v", chunkURL, err)
		return nil, err
	}
	mountlog.Debugf("chunk GET ok url=%s bytes=%d in %s", chunkURL, len(data), time.Since(start))
	return data, nil
}

func (t *HTTPTransport) DeleteChunk(ctx context.Context, nodeURL, chunkID string) error {
	url := t.chunkURL(nodeURL, chunkID, false)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}
