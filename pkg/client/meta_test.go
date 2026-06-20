package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRemoteMetaAppliesURIPrefixToNodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/memoryfs/v1/cluster/overview":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"replica_factor":2}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	meta := NewRemoteMeta([]string{srv.URL})
	nodes := meta.Nodes()
	if len(nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(nodes))
	}
	want := srv.URL + "/memoryfs"
	if nodes[0] != want {
		t.Fatalf("nodes[0] = %q, want %q", nodes[0], want)
	}
}
