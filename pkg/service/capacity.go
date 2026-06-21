package service

import "strings"

// SumDiskUsageBytes totals disk+mem cache usage for reachable nodes.
// nodeURLs filters by normalized host (empty = all reachable nodes).
func SumDiskUsageBytes(ov ClusterOverview, nodeURLs []string) uint64 {
	want := nodeURLSet(nodeURLs)
	var used uint64
	for _, node := range ov.Nodes {
		if !node.Reachable {
			continue
		}
		if len(want) > 0 {
			if _, ok := want[normalizeNodeURL(node.URL)]; !ok {
				continue
			}
		}
		used += uint64(node.Stats.DiskBytes + node.Stats.MemCacheBytes)
	}
	return used
}

// SumDiskQuotaBytes totals configured disk quota for reachable nodes.
// nodeURLs filters by normalized host (empty = all reachable nodes).
func SumDiskQuotaBytes(ov ClusterOverview, nodeURLs []string) uint64 {
	want := nodeURLSet(nodeURLs)
	var total uint64
	for _, node := range ov.Nodes {
		if !node.Reachable {
			continue
		}
		if len(want) > 0 {
			if _, ok := want[normalizeNodeURL(node.URL)]; !ok {
				continue
			}
		}
		if node.Stats.DiskQuotaBytes > 0 {
			total += uint64(node.Stats.DiskQuotaBytes)
		}
	}
	return total
}

func nodeURLSet(nodeURLs []string) map[string]struct{} {
	want := make(map[string]struct{}, len(nodeURLs))
	for _, n := range nodeURLs {
		if key := normalizeNodeURL(n); key != "" {
			want[key] = struct{}{}
		}
	}
	return want
}

func normalizeNodeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	if i := strings.Index(raw, "/"); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimRight(raw, "/")
}
