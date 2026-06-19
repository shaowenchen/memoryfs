package chunk_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
)

func TestDiskStorePersist(t *testing.T) {
	dir := t.TempDir()
	store, err := chunk.NewDiskStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("1_0", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	data, ok := store.Get("1_0")
	if !ok || string(data) != "hello" {
		t.Fatalf("get: %v %q", ok, data)
	}

	// simulate restart
	store2, err := chunk.NewDiskStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, ok = store2.Get("1_0")
	if !ok || string(data) != "hello" {
		t.Fatalf("persist: %v %q", ok, data)
	}
	if _, err := os.Stat(filepath.Join(dir, "1_", "1_0")); err != nil {
		t.Fatalf("file on disk: %v", err)
	}
}

func TestSelectNodesRF(t *testing.T) {
	nodes := []string{"http://a", "http://b", "http://c"}
	for rf := 1; rf <= 3; rf++ {
		out, err := chunk.SelectNodes(nodes, "2_0", rf)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != rf {
			t.Fatalf("rf=%d got %d nodes", rf, len(out))
		}
	}
}
