package cluster

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestWaitRaftTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := waitRaftTCP(ctx, ln.Addr().String(), time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitRaftTCPTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := waitRaftTCP(ctx, "127.0.0.1:1", 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout")
	}
}
