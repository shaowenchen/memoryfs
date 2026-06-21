package service

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/cluster"
	"github.com/shaowenchen/memoryfs/pkg/lifecycle"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/raftnode"
)

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func waitLeader(t *testing.T, rn *raftnode.Node) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !rn.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !rn.IsLeader() {
		t.Fatal("node did not become leader")
	}
}

func TestWriteBlockLeaderStoresAndUpdatesAttr(t *testing.T) {
	dir := t.TempDir()
	raftAddr := freeTCPAddr(t)
	rn, err := raftnode.Start(raftnode.Config{
		ID: "n0", RaftAddr: raftAddr, AdvertiseRaft: raftAddr, DataDir: dir + "/0",
		Bootstrap: true, HTTPAddr: "http://127.0.0.1:19800",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rn.Close() }()
	waitLeader(t, rn)
	if err := cluster.NewMembership(rn, "n0").RegisterSelf(cluster.Member{
		ID: "n0", HTTP: "http://127.0.0.1:19800", Raft: raftAddr,
	}); err != nil {
		t.Fatal(err)
	}

	store, err := meta.NewLocalStore(rn.KV())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureRoot(context.Background()); err != nil {
		t.Fatal(err)
	}
	root, _ := store.GetAttr(context.Background(), meta.RootIno())
	attr, err := store.Create(context.Background(), root.Ino, "f", 0o644, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	chunks := chunk.NewMemoryStore()
	svc := New(Config{
		NodeID: "n0", NodeHTTP: "http://127.0.0.1:19800", RaftNode: rn, Meta: store,
		Chunks: chunks, Registry: chunk.NewRegistry(rn.KV()), Lifecycle: lifecycle.NewManager(),
		ReplicaFactor: 1,
	})

	payload := []byte("block-data")
	got, err := svc.WriteBlock(context.Background(), attr.Ino, 0, 0, payload, uint64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if got.Size != uint64(len(payload)) {
		t.Fatalf("size: got %d want %d", got.Size, len(payload))
	}
	if data, ok := chunks.Get(meta.BlockID(attr.Ino, 0, 0)); !ok || string(data) != "block-data" {
		t.Fatalf("chunk stored: %q ok=%v", data, ok)
	}
	loc, err := svc.GetChunkRegistry(context.Background(), meta.BlockID(attr.Ino, 0, 0))
	if err != nil || len(loc.Replicas) == 0 {
		t.Fatalf("registry: loc=%v err=%v", loc, err)
	}
}
