package service

import (
	"context"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/kv"
	"github.com/shaowenchen/memoryfs/pkg/lifecycle"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/raftnode"
	"github.com/shaowenchen/memoryfs/pkg/transport"
)

func testStandaloneService(t *testing.T) *Service {
	t.Helper()
	rn, err := raftnode.Start(raftnode.Config{Standalone: true, ID: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rn.Close() })
	metaStore, err := meta.NewLocalStore(rn.KV())
	if err != nil {
		t.Fatal(err)
	}
	return New(Config{
		NodeID:        "n1",
		NodeHTTP:      "http://127.0.0.1:8080",
		RaftNode:      rn,
		Meta:          metaStore,
		Chunks:        chunk.NewMemoryStore(),
		Registry:      chunk.NewRegistry(rn.KV()),
		Lifecycle:     lifecycle.NewManager(),
		Transport:     transport.NewHTTPTransport(),
		ReplicaFactor: 2,
	})
}

func TestRecordChunkRegistryStandalone(t *testing.T) {
	svc := testStandaloneService(t)
	if err := svc.RecordChunkRegistry(context.Background(), "1_0", []string{"http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("record registry: %v", err)
	}
	loc, err := svc.cfg.Registry.Get("1_0")
	if err != nil {
		t.Fatalf("get registry: %v", err)
	}
	if len(loc.Replicas) != 1 {
		t.Fatalf("replicas: %v", loc.Replicas)
	}
}

func TestLoadClusterConfigReplicaFactor(t *testing.T) {
	svc := testStandaloneService(t)
	if err := svc.cfg.RaftNode.KV().Set(configRFKey, []byte("3")); err != nil {
		t.Fatal(err)
	}
	svc.LoadClusterConfig()
	if svc.ReplicaFactor() != 3 {
		t.Fatalf("expected rf=3, got %d", svc.ReplicaFactor())
	}
}

func TestClusterEpochPersisted(t *testing.T) {
	svc := testStandaloneService(t)
	if _, err := svc.cfg.RaftNode.KV().Incr(clusterEpochKey); err != nil {
		t.Fatal(err)
	}
	_, _, _, epoch, _ := svc.Health()
	if epoch != 1 {
		t.Fatalf("expected epoch 1, got %d", epoch)
	}
}

func TestApplyRegistrySetRequiresLeader(t *testing.T) {
	svc := testStandaloneService(t)
	if err := svc.ApplyRegistrySet(context.Background(), "2_0", []string{"http://n1"}, 1); err != nil {
		t.Fatalf("standalone is leader: %v", err)
	}
}

func TestMemoryKVIncrEpoch(t *testing.T) {
	store := kv.NewMemoryKV()
	v, err := store.Incr(clusterEpochKey)
	if err != nil || v != 1 {
		t.Fatalf("incr: v=%d err=%v", v, err)
	}
}
