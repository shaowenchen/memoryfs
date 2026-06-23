package service

import "testing"

func TestSumDiskQuotaAndUsage(t *testing.T) {
	ov := ClusterOverview{
		Nodes: []NodeOverview{
			{URL: "http://n1:19800", Reachable: true, Stats: Stats{DiskBytes: 1 << 30, DiskQuotaBytes: 32 << 30, MemBytes: 100}},
			{URL: "http://n2:19800", Reachable: true, Stats: Stats{DiskBytes: 2 << 30, DiskQuotaBytes: 32 << 30}},
			{URL: "http://n3:19800", Reachable: false, Stats: Stats{DiskBytes: 9 << 30, DiskQuotaBytes: 32 << 30}},
		},
	}
	filter := []string{"http://n1:19800/memoryfs", "http://n2:19800"}

	if got := SumDiskQuotaBytes(ov, filter); got != 64<<30 {
		t.Fatalf("quota: got %d want %d", got, 64<<30)
	}
	if got := SumDiskUsageBytes(ov, filter); got != (1<<30)+(2<<30)+100 {
		t.Fatalf("usage: got %d", got)
	}
	if got := SumDiskQuotaBytes(ov, nil); got != 64<<30 {
		t.Fatalf("quota all reachable: got %d", got)
	}
}
