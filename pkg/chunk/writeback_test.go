package chunk_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
)

func TestWriteBackStoreFlush(t *testing.T) {
	dir := t.TempDir()
	disk, err := chunk.NewDiskStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := chunk.NewWriteBackStore(disk)

	if err := store.Put("1_0", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, ok := disk.Get("1_0"); ok {
		t.Fatal("expected write-back chunk to stay in memory before flush")
	}

	n, err := store.Flush()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if n != 1 {
		t.Fatalf("flush count: got %d want 1", n)
	}

	data, ok := disk.Get("1_0")
	if !ok || string(data) != "hello" {
		t.Fatalf("disk after flush: %v %q", ok, data)
	}

	store2 := chunk.NewWriteBackStore(disk)
	data, ok = store2.Get("1_0")
	if !ok || string(data) != "hello" {
		t.Fatalf("reload: %v %q", ok, data)
	}
}

func TestDiskStoreFlush(t *testing.T) {
	dir := t.TempDir()
	store, err := chunk.NewDiskStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("2_0", []byte("sync")); err != nil {
		t.Fatal(err)
	}
	n, err := store.Flush()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if n != 1 {
		t.Fatalf("flush count: got %d want 1", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "2_", "2_0")); err != nil {
		t.Fatalf("file on disk: %v", err)
	}
}
