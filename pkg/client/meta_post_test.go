package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/meta"
)

func TestPostLookupNotFoundDoesNotFanOut(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(fsResp{Error: "entry not found"})
	}))
	defer srv.Close()

	rm := &RemoteMeta{
		nodes:  []string{srv.URL, "http://127.0.0.1:9"},
		client: &http.Client{Timeout: 2 * time.Second},
	}
	_, err := rm.Lookup(context.Background(), meta.RootIno(), "missing.txt")
	if err == nil || !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("lookup err = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("lookup fan-out calls = %d, want 1", got)
	}
}

func TestSetLeaderPinsWrites(t *testing.T) {
	var leaderCalls atomic.Int32
	leader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaderCalls.Add(1)
		_ = json.NewEncoder(w).Encode(fsResp{Attr: &meta.Attr{Ino: 2, Mode: 0o100644}})
	}))
	defer leader.Close()
	follower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("write should not hit follower once leader is pinned")
	}))
	defer follower.Close()

	rm := &RemoteMeta{
		nodes:  []string{follower.URL, leader.URL},
		client: &http.Client{Timeout: 2 * time.Second},
	}
	rm.SetLeader(leader.URL)
	if _, err := rm.Create(context.Background(), meta.RootIno(), "a.txt", 0o644, 0, 0); err != nil {
		t.Fatal(err)
	}
	if got := leaderCalls.Load(); got != 1 {
		t.Fatalf("leader calls = %d, want 1", got)
	}
}
