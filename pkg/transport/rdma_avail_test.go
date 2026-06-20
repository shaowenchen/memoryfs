package transport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRDMADevicesPresent(t *testing.T) {
	dir := t.TempDir()
	if rdmaDevicesPresent(dir) {
		t.Fatal("empty dir should not have RDMA")
	}
	ib := filepath.Join(dir, "infiniband")
	if err := os.Mkdir(ib, 0o755); err != nil {
		t.Fatal(err)
	}
	if rdmaDevicesPresent(ib) {
		t.Fatal("dir without uverbs should not have RDMA")
	}
	if err := os.WriteFile(filepath.Join(ib, "uverbs0"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !rdmaDevicesPresent(ib) {
		t.Fatal("expected RDMA when uverbs device exists")
	}
}
