package service

import "testing"

func TestSummarizeClusterStorage(t *testing.T) {
	got := summarizeClusterStorage([]NodeOverview{
		{Reachable: true, Stats: Stats{DiskBytes: 1 << 30, MemCacheBytes: 100, DiskQuotaBytes: 32 << 30}},
		{Reachable: true, Stats: Stats{DiskBytes: 2 << 30, DiskQuotaBytes: 32 << 30}},
		{Reachable: false, Stats: Stats{DiskQuotaBytes: 32 << 30}},
	})
	if got.TotalNodes != 3 || got.ReachableNodes != 2 {
		t.Fatalf("nodes: %+v", got)
	}
	if got.TotalDiskQuotaBytes != 64<<30 {
		t.Fatalf("quota: got %d want %d", got.TotalDiskQuotaBytes, 64<<30)
	}
	if got.TotalDiskBytes != 3<<30 || got.TotalMemCacheBytes != 100 {
		t.Fatalf("usage: %+v", got)
	}
}
