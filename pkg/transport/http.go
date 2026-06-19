package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http put chunk: %s", body)
	}
	return nil
}

func (t *HTTPTransport) GetChunk(ctx context.Context, nodeURL, chunkID string) ([]byte, error) {
	url := strings.TrimRight(nodeURL, "/") + "/chunks/" + chunkID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("chunk not found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http get chunk: %s", body)
	}
	return io.ReadAll(resp.Body)
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
	defer resp.Body.Close()
	return nil
}
