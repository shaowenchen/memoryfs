package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/shaowenchen/memoryfs/pkg/service"
)

// StatusOptions configures cluster status output.
type StatusOptions struct {
	Seed      string
	URIPrefix string
	Token     string
	JSON      bool
	AutoPrefix bool
}

// PrintStatus prints cluster storage and node status.
func PrintStatus(ctx context.Context, opt StatusOptions) error {
	prefix := opt.URIPrefix
	if opt.AutoPrefix && prefix == "" {
		prefix = DetectPrefix(ctx, opt.Seed, opt.Token)
	}
	c := NewClient(opt.Seed, prefix, opt.Token)
	ov, err := c.Overview(ctx)
	if err != nil {
		return err
	}
	if opt.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ov)
	}
	printStatusText(os.Stdout, opt.Seed, prefix, ov)
	return nil
}

func printStatusText(w io.Writer, seed, prefix string, ov *service.ClusterOverview) {
	_, _ = fmt.Fprintf(w, "MemoryFS Cluster Status\n")
	_, _ = fmt.Fprintf(w, "Seed:   %s\n", seed)
	if prefix != "" {
		_, _ = fmt.Fprintf(w, "Prefix: %s\n", prefix)
	}
	_, _ = fmt.Fprintf(w, "Leader: %s\n", valueOr(ov.Leader, "-"))
	_, _ = fmt.Fprintf(w, "Epoch:  %d   RF: %d   Repair pending: %d   Local node: %s\n",
		ov.ClusterEpoch, ov.ReplicaFactor, ov.Repair.Pending, valueOr(ov.NodeID, "-"))
	_, _ = fmt.Fprintf(w, "Cluster: %d/%d nodes reachable   quota %s   used %s (disk %s + mem %s)\n\n",
		ov.Storage.ReachableNodes,
		ov.Storage.TotalNodes,
		FormatBytes(ov.Storage.TotalDiskQuotaBytes),
		FormatBytes(ov.Storage.TotalDiskBytes+ov.Storage.TotalMemCacheBytes),
		FormatBytes(ov.Storage.TotalDiskBytes),
		FormatBytes(ov.Storage.TotalMemCacheBytes),
	)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tROLE\tSTATE\tCHUNKS\tDISK\tMEM CACHE\tEPOCH\tREACH")
	for _, n := range ov.Nodes {
		role := n.Role
		state := n.NodeState
		if !n.Reachable {
			role = "-"
			state = "down"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%d\t%t\n",
			n.URL,
			valueOr(role, "-"),
			valueOr(state, "-"),
			n.Stats.ChunkCount,
			FormatBytes(n.Stats.DiskBytes),
			FormatBytes(n.Stats.MemCacheBytes),
			n.ClusterEpoch,
			n.Reachable,
		)
	}
	_ = tw.Flush()
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// FormatBytes renders a human-readable size.
func FormatBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
