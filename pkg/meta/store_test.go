package meta_test

import (
	"context"
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/kv"
	"github.com/shaowenchen/memoryfs/pkg/meta"
)

func TestLocalStoreFileOps(t *testing.T) {
	store, err := meta.NewLocalStore(kv.NewMemoryKV())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	attr, err := store.Create(ctx, meta.RootIno(), "hello.txt", 0o644, 1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if attr.Ino == 0 {
		t.Fatal("expected inode")
	}

	found, err := store.Lookup(ctx, meta.RootIno(), "hello.txt")
	if err != nil || found.Ino != attr.Ino {
		t.Fatalf("lookup: %v %+v", err, found)
	}

	if _, err := store.Mkdir(ctx, meta.RootIno(), "dir", 0o755, 1000, 1000); err != nil {
		t.Fatal(err)
	}
	if err := store.Rename(ctx, meta.RootIno(), meta.RootIno(), "hello.txt", "world.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lookup(ctx, meta.RootIno(), "world.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStoreSymlink(t *testing.T) {
	store, err := meta.NewLocalStore(kv.NewMemoryKV())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	attr, err := store.Symlink(ctx, meta.RootIno(), "link", "/etc/passwd", 1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if attr.Target != "/etc/passwd" {
		t.Fatalf("target: %q", attr.Target)
	}
}
