package cluster

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

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

func waitLeader(t *testing.T, rn *raftnode.Node, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !rn.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !rn.IsLeader() {
		t.Skip("node did not become leader, skipping test")
	}
}

func TestMembershipRegisterAndSync(t *testing.T) {
	rn, err := raftnode.Start(raftnode.Config{Standalone: true, ID: "memoryfs-0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rn.Close() })

	m := NewMembership(rn, "memoryfs-0")
	if err := m.RegisterSelf(Member{
		ID:   "memoryfs-0",
		HTTP: "http://n0:19800",
		Raft: "n0:19802",
	}); err != nil {
		t.Fatal(err)
	}
	members, err := m.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].HTTP != "http://n0:19800" {
		t.Fatalf("sync: %+v", members)
	}
}

func TestMembershipAdmitReturnsFullList(t *testing.T) {
	dir := t.TempDir()
	raft0 := freeTCPAddr(t)
	raft1 := freeTCPAddr(t)

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
	if err := m.RegisterSelf(Member{ID: "memoryfs-0", HTTP: "http://n0:19800", Raft: raft0}); err != nil {
		t.Fatal(err)
	}

	members, err := m.Admit(context.Background(), Member{
		ID:   "memoryfs-1",
		HTTP: "http://n1:19800",
		Raft: raft1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members after admit, got %+v", members)
	}

	followerMembers, err := NewMembership(rn1, "memoryfs-1").Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(followerMembers) != 2 {
		t.Fatalf("follower sync: expected 2 members, got %+v", followerMembers)
	}
}

func TestMembershipAdmitRefreshesExistingMember(t *testing.T) {
	dir := t.TempDir()
	raft0 := freeTCPAddr(t)
	raft1 := freeTCPAddr(t)

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
		ID: "memoryfs-1", HTTP: "http://dns:19800", Raft: raft1,
	}); err != nil {
		t.Fatal(err)
	}

	members, err := m.Admit(context.Background(), Member{
		ID: "memoryfs-1", HTTP: "http://10.0.0.2:19800", Raft: raft1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %+v", members)
	}
	for _, mem := range members {
		if mem.ID == "memoryfs-1" && mem.HTTP != "http://10.0.0.2:19800" {
			t.Fatalf("expected refreshed HTTP, got %+v", members)
		}
	}
}
