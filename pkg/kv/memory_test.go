package kv_test

import (
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/kv"
)

func TestMemoryKVBasic(t *testing.T) {
	store := kv.NewMemoryKV()
	if err := store.Set("k", []byte("v")); err != nil {
		t.Fatal(err)
	}
	v, err := store.Get("k")
	if err != nil || string(v) != "v" {
		t.Fatalf("get: %v %q", err, v)
	}
	n, err := store.Incr("counter")
	if err != nil || n != 1 {
		t.Fatalf("incr: %v %d", err, n)
	}
	if err := store.SAdd("set", "a", "b"); err != nil {
		t.Fatal(err)
	}
	members, err := store.SMembers("set")
	if err != nil || len(members) != 2 {
		t.Fatalf("smembers: %v %v", err, members)
	}
}

func TestMemoryKVBatch(t *testing.T) {
	store := kv.NewMemoryKV()
	err := store.Batch([]kv.Op{
		{Type: kv.OpHSet, Key: "d", Field: "name", Value: []byte("1")},
		{Type: kv.OpSet, Key: "ino:1", Value: []byte("meta")},
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := store.HGet("d", "name")
	if err != nil || string(v) != "1" {
		t.Fatalf("hget: %v", err)
	}
}

func TestMemoryKVSnapshot(t *testing.T) {
	a := kv.NewMemoryKV()
	_ = a.Set("x", []byte("1"))
	raw, err := a.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	b := kv.NewMemoryKV()
	if err := b.Restore(raw); err != nil {
		t.Fatal(err)
	}
	v, err := b.Get("x")
	if err != nil || string(v) != "1" {
		t.Fatalf("restore: %v", err)
	}
}
