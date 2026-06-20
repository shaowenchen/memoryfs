package cli_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/cli"
)

func TestOverview(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cluster/overview" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"overview":{"node_id":"n1","leader":"http://n1","cluster_epoch":1,"replica_factor":2,"repair":{"pending":1},"nodes":[]}}`))
	}))
	defer srv.Close()

	c := cli.NewClient(srv.URL, "", "")
	ov, err := c.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ov.NodeID != "n1" || ov.Repair.Pending != 1 {
		t.Fatalf("overview: %+v", ov)
	}
}

func TestDetectPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/memoryfs/v1/cluster/overview" {
			_, _ = w.Write([]byte(`{"overview":{"node_id":"n1","nodes":[]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	if got := cli.DetectPrefix(context.Background(), srv.URL, ""); got != "/memoryfs" {
		t.Fatalf("prefix: %q", got)
	}
}

func TestFormatBytes(t *testing.T) {
	if cli.FormatBytes(0) != "0 B" {
		t.Fatal(cli.FormatBytes(0))
	}
	if !strings.HasPrefix(cli.FormatBytes(2048), "2.0") {
		t.Fatal(cli.FormatBytes(2048))
	}
}

func TestBenchmarkRoundTrip(t *testing.T) {
	store := map[string][]byte{}
	var baseURL string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/cluster/overview":
			_, _ = w.Write([]byte(`{"overview":{"node_id":"n1","leader":"` + baseURL + `","cluster_epoch":1,"replica_factor":1,"repair":{},"nodes":[]}}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/chunks/"):
			id := strings.TrimPrefix(r.URL.Path, "/chunks/")
			body, _ := io.ReadAll(r.Body)
			store[id] = body
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/chunks/"):
			id := strings.TrimPrefix(r.URL.Path, "/chunks/")
			data, ok := store[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(data)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/chunks/"):
			id := strings.TrimPrefix(r.URL.Path, "/chunks/")
			delete(store, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()
	baseURL = srv.URL

	res, err := cli.RunBenchmark(context.Background(), cli.BenchmarkOptions{
		Seed: srv.URL, Size: 4096, Writes: 4, Reads: 4, Workers: 2, Cleanup: true,
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.WriteMBs <= 0 || res.ReadMBs <= 0 {
		t.Fatalf("throughput: write=%.2f read=%.2f", res.WriteMBs, res.ReadMBs)
	}
	if len(store) != 0 {
		t.Fatalf("cleanup left chunks: %d", len(store))
	}
}
