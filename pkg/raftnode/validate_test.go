package raftnode

import (
	"net"
	"testing"
	"time"

	"github.com/hashicorp/raft"
)

func TestAddrPort(t *testing.T) {
	tests := map[string]string{
		":19802":                                              "19802",
		"127.0.0.1:19802":                                     "19802",
		"memoryfs-0.headless.default.svc.cluster.local:19802": "19802",
		"10.0.0.1:8081":                                       "8081",
		"":                                                    "",
	}
	for in, want := range tests {
		if got := addrPort(in); got != want {
			t.Fatalf("addrPort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidatePeerPortsRejectsStalePort(t *testing.T) {
	err := validatePeerPorts([]raft.Server{
		{ID: "memoryfs-0", Address: "10.0.0.1:19802"},
		{ID: "memoryfs-1", Address: "10.0.0.2:8081"},
	}, "19802", "/data/memoryfs/abc/memoryfs-0")
	if err == nil {
		t.Fatal("expected stale port validation error")
	}
}

func TestValidatePeerPortsAllowsMatchingPort(t *testing.T) {
	err := validatePeerPorts([]raft.Server{
		{ID: "memoryfs-0", Address: "10.0.0.1:19802"},
		{ID: "memoryfs-1", Address: "10.0.0.2:19802"},
	}, "19802", "/data")
	if err != nil {
		t.Fatal(err)
	}
}

func TestStartRejectsStalePersistedConfig(t *testing.T) {
	dir := t.TempDir()
	raft0 := freeTCPAddr(t)

	rn, err := Start(Config{
		ID:            "memoryfs-0",
		Bootstrap:     true,
		DataDir:       dir,
		RaftAddr:      raft0,
		AdvertiseRaft: raft0,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !rn.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !rn.IsLeader() {
		_ = rn.Close()
		t.Fatal("bootstrap node did not become leader")
	}
	future := rn.raft.AddVoter(raft.ServerID("memoryfs-1"), "127.0.0.1:8081", 0, 0)
	if err := future.Error(); err != nil {
		_ = rn.Close()
		t.Fatal(err)
	}
	_ = rn.Close()

	raft1 := freeTCPAddr(t)
	_, err = Start(Config{
		ID:            "memoryfs-0",
		DataDir:       dir,
		RaftAddr:      raft1,
		AdvertiseRaft: raft1,
	})
	if err == nil {
		t.Fatal("expected restart to reject stale raft configuration")
	}
}

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
