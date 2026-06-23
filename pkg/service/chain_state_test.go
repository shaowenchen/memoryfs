package service

import (
	"context"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
)

func TestSyncStartAndDoneTransitionState(t *testing.T) {
	svc := testStandaloneService(t)
	st := svc.GetChainState(context.Background(), 0)
	if st.State != chunk.TargetStateServing && st.State != chunk.TargetStateOffline {
		t.Fatalf("unexpected initial state: %+v", st)
	}
	start, err := svc.SyncStart(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("sync start: %v", err)
	}
	if start.ChainID != 1 {
		t.Fatalf("unexpected chain id: %+v", start)
	}
	st = svc.GetChainState(context.Background(), 1)
	if st.State != chunk.TargetStateSyncing {
		t.Fatalf("expected syncing, got %+v", st)
	}
	if err := svc.SyncDone(context.Background(), 1, 11); err != nil {
		t.Fatalf("sync done: %v", err)
	}
	st = svc.GetChainState(context.Background(), 1)
	if st.State != chunk.TargetStateServing {
		t.Fatalf("expected serving, got %+v", st)
	}
	if st.ChainVer < 11 {
		t.Fatalf("expected chain version >= 11, got %+v", st)
	}
}

