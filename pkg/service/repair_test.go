package service

import (
	"context"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/ports"
)

func TestRepairQueueEnqueueAndRun(t *testing.T) {
	svc := testStandaloneService(t)
	_ = svc.cfg.Chunks.Put("1_0", []byte("payload"))
	svc.enqueueRepair("1_0", []string{ports.DefaultHTTPURL(), "http://127.0.0.1:19804"})
	if svc.RepairInfo(0).Pending != 1 {
		t.Fatalf("expected 1 pending")
	}
	_, failed := svc.RunRepair(context.Background())
	if failed != 1 {
		t.Fatalf("expected 1 failed repair without peers, got failed=%d pending=%d", failed, svc.RepairInfo(0).Pending)
	}
}

func TestRepairQueueDone(t *testing.T) {
	rq := NewRepairQueue()
	rq.Enqueue("x", []string{"a"})
	rq.Done("x")
	if rq.Len() != 0 {
		t.Fatal("queue should be empty")
	}
}

func TestClusterOverviewLocal(t *testing.T) {
	svc := testStandaloneService(t)
	ov := svc.ClusterOverview(context.Background())
	if ov.NodeID != "n1" {
		t.Fatalf("node id: %s", ov.NodeID)
	}
	if len(ov.Nodes) == 0 {
		t.Fatal("expected at least local node")
	}
}
