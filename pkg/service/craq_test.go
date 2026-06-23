package service

import (
	"context"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
)

func TestGetChunkRejectsPendingVersion(t *testing.T) {
	svc := testStandaloneService(t)
	if err := svc.cfg.Chunks.Put("c1", []byte("abc")); err != nil {
		t.Fatal(err)
	}
	svc.setMeta(chunk.ChunkMeta{
		ChunkID:   "c1",
		UpdateVer: 2,
		CommitVer: 1,
		State:     chunk.ChunkStatePending,
	})
	if _, err := svc.GetChunk(context.Background(), "c1"); err == nil {
		t.Fatalf("expected pending chunk read to fail")
	}
}

func TestStoreChunkLocalCommitsWhenNoSuccessor(t *testing.T) {
	svc := testStandaloneService(t)
	err := svc.StoreChunkLocal(context.Background(), "c2", []byte("hello"), ReplicaWrite{
		Stage:     stagePrepare,
		ChainID:   0,
		ChainVer:  1,
		UpdateVer: 7,
		Replicas:  []string{svc.cfg.NodeHTTP},
	})
	if err != nil {
		t.Fatalf("store chunk local: %v", err)
	}
	meta := svc.getMeta("c2")
	if !meta.Committed() {
		t.Fatalf("expected committed meta, got %+v", meta)
	}
	if meta.UpdateVer != 7 || meta.CommitVer != 7 {
		t.Fatalf("expected version 7 committed, got %+v", meta)
	}
}

func TestDeleteChunkRemovesDataAndAdvancesVersion(t *testing.T) {
	svc := testStandaloneService(t)
	if _, err := svc.PutChunk(context.Background(), "c3", []byte("payload")); err != nil {
		t.Fatalf("put chunk: %v", err)
	}
	before := svc.getMeta("c3")
	if err := svc.DeleteChunk(context.Background(), "c3"); err != nil {
		t.Fatalf("delete chunk: %v", err)
	}
	if _, ok := svc.cfg.Chunks.Get("c3"); ok {
		t.Fatalf("expected chunk to be removed")
	}
	after := svc.getMeta("c3")
	if after.UpdateVer <= before.UpdateVer {
		t.Fatalf("expected update version to advance: before=%+v after=%+v", before, after)
	}
}
