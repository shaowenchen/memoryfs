package cluster

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/raftnode"
)

func TestEnsureRaftVoterUpdatesAddress(t *testing.T) {
	dir := t.TempDir()
	raft0 := freeTCPAddr(t)
	raft1 := freeTCPAddr(t)
	updated := freeTCPAddr(t)

	rn0, err := raftnode.Start(raftnode.Config{
		ID:            "memoryfs-0",
		Bootstrap:     true,
		DataDir:       filepath.Join(dir, "0"),
		RaftAddr:      raft0,
		AdvertiseRaft: raft0,
		HTTPAddr:      "http://n0:19800",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rn0.Close() })
	waitLeader(t, rn0, 5*time.Second)

	rn1, err := raftnode.Start(raftnode.Config{
		ID:            "memoryfs-1",
		DataDir:       filepath.Join(dir, "1"),
		RaftAddr:      raft1,
		AdvertiseRaft: raft1,
		HTTPAddr:      "http://n1:19800",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rn1.Close() })

	m := NewMembership(rn0, "memoryfs-0")
	if err := m.RegisterSelf(Member{ID: "memoryfs-0", HTTP: "http://10.0.0.1:19800", Raft: raft0}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Admit(context.Background(), Member{
		ID: "memoryfs-1", HTTP: "http://10.0.0.2:19800", Raft: raft1,
	}); err != nil {
		t.Fatal(err)
	}

	if err := m.ensureRaftVoter(Member{ID: "memoryfs-1", Raft: updated}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := rn0.ServerAddress("memoryfs-1")
	if err != nil || !ok {
		t.Fatalf("server address: ok=%v err=%v", ok, err)
	}
	if got != updated {
		t.Fatalf("raft addr = %q, want %q", got, updated)
	}
}

func TestIsKubernetesDNS(t *testing.T) {
	if !isKubernetesDNS("memoryfs-0.headless.memoryfs.svc.cluster.local:19802") {
		t.Fatal("expected k8s dns")
	}
	if isKubernetesDNS("10.0.0.1:19802") {
		t.Fatal("expected ip")
	}
}
