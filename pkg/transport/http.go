package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/mountlog"
)

// HTTPTransport uses REST chunk endpoints.
type HTTPTransport struct {
	client *http.Client
}

// NewHTTPTransport creates an HTTP chunk transport.
func NewHTTPTransport() *HTTPTransport {
	return &HTTPTransport{client: &http.Client{Timeout: 30 * time.Second}}
}

func (t *HTTPTransport) Kind() Kind { return KindHTTP }

func (t *HTTPTransport) PutChunk(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	return t.putChunk(ctx, nodeURL, chunkID, data, false)
}

func (t *HTTPTransport) PutChunkReplica(ctx context.Context, nodeURL, chunkID string, data []byte) error {
	return t.putChunk(ctx, nodeURL, chunkID, data, true)
}

func (t *HTTPTransport) putChunk(ctx context.Context, nodeURL, chunkID string, data []byte, replica bool) error {
	url := strings.TrimRight(nodeURL, "/") + "/chunks/" + chunkID
	if replica {
		url += "?replica=1"
	}
	mountlog.Debugf("chunk PUT start url=%s bytes=%d replica=%v", url, len(data), replica)
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		mountlog.Errorf("chunk PUT url=%s bytes=%d: %v", url, len(data), err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("http put chunk status=%d body=%s", resp.StatusCode, body)
		mountlog.Errorf("chunk PUT url=%s bytes=%d: %v", url, len(data), err)
		return err
	}
	mountlog.Debugf("chunk PUT ok url=%s bytes=%d status=%d in %s", url, len(data), resp.StatusCode, time.Since(start))
	return nil
}

func (t *HTTPTransport) GetChunk(ctx context.Context, nodeURL, chunkID string) ([]byte, error) {
	url := strings.TrimRight(nodeURL, "/") + "/chunks/" + chunkID
	mountlog.Debugf("chunk GET start url=%s", url)
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		mountlog.Errorf("chunk GET url=%s: %v", url, err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		mountlog.Debugf("chunk GET miss url=%s in %s", url, time.Since(start))
		return nil, fmt.Errorf("chunk not found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("http get chunk status=%d body=%s", resp.StatusCode, body)
		mountlog.Errorf("chunk GET url=%s: %v", url, err)
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		mountlog.Errorf("chunk GET read body url=%s: %v", url, err)
		return nil, err
	}
	mountlog.Debugf("chunk GET ok url=%s bytes=%d in %s", url, len(data), time.Since(start))
	return data, nil
}

func (t *HTTPTransport) DeleteChunk(ctx context.Context, nodeURL, chunkID string) error {
	url := strings.TrimRight(nodeURL, "/") + "/chunks/" + chunkID
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
