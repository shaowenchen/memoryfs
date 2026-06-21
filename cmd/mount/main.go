//go:build !windows

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/shaowenchen/memoryfs/pkg/ports"
	"github.com/shaowenchen/memoryfs/pkg/cli"
	"github.com/shaowenchen/memoryfs/pkg/client"
	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/fusefs"
	"github.com/shaowenchen/memoryfs/pkg/mountlog"
	"github.com/shaowenchen/memoryfs/pkg/service"
	"github.com/shaowenchen/memoryfs/pkg/storage"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	mountPoint := flag.String("mount", "", "mount point (required)")
	nodes := flag.String("nodes", envOr("MEMORYFS_NODES", ""), "comma-separated node HTTP URLs (required)")
	foreground := flag.Bool("f", false, "run in foreground")
	debug := flag.Bool("debug", false, "enable fuse debug")
	verbose := flag.Bool("v", false, "verbose: log I/O operations and periodic heartbeats")
	flag.Parse()

	mountlog.SetVerbose(*verbose)

	if *mountPoint == "" {
		log.Fatal("mount point is required: -mount /path/to/mount")
	}
	if *nodes == "" {
		log.Fatal("nodes is required: -nodes " + ports.DefaultHTTPURL())
	}

	var nodeList []string
	for _, n := range strings.Split(*nodes, ",") {
		if n = strings.TrimSpace(n); n != "" {
			nodeList = append(nodeList, n)
		}
	}

	log.Printf("memoryfs mount starting pid=%d mount=%s nodes=%v verbose=%v", os.Getpid(), *mountPoint, nodeList, *verbose)

	metaStore := client.NewRemoteMeta(nodeList)
	defer func() { _ = metaStore.Close() }()

	seed := strings.TrimRight(strings.TrimSpace(nodeList[0]), "/")
	prefix := cli.NormalizePrefix(cli.DetectPrefix(context.Background(), seed, ""))
	log.Printf("detected uri prefix=%q", prefix)
	apiClient := cli.NewClient(seed, prefix, "")

	ov, err := apiClient.Overview(context.Background())
	if err != nil {
		log.Fatalf("cluster unreachable at %s: %v", seed, err)
	}
	log.Printf("cluster ok: leader=%s rf=%d epoch=%d reachable_nodes=%d",
		ov.Leader, ov.ReplicaFactor, ov.ClusterEpoch, reachableNodes(ov))
	if ov.Leader != "" {
		metaStore.SetLeader(prefixedLeaderURL(ov.Leader, prefix))
		log.Printf("meta leader pinned: %s", ov.Leader)
	}
	if _, err := metaStore.ListNodes(context.Background()); err != nil {
		log.Printf("warning: refresh meta nodes: %v", err)
	}
	for _, n := range ov.Nodes {
		log.Printf("  node url=%s reachable=%v role=%s state=%s chunks=%d disk=%d",
			n.URL, n.Reachable, n.Role, n.NodeState, n.Stats.ChunkCount, n.Stats.DiskBytes)
	}

	rf := ov.ReplicaFactor
	if rf <= 0 {
		rf = detectReplicaFactor(nodeList)
	}
	chunkNodes := metaStore.Nodes()
	chunks := storage.NewHTTPChunkStore(metaStore, chunkNodes, rf, prefix)
	if err := chunks.RefreshNodes(context.Background()); err != nil {
		log.Printf("warning: refresh nodes: %v", err)
	}
	if len(chunks.Nodes()) == 0 {
		log.Fatal("no cluster nodes available; check -nodes URLs")
	}
	log.Printf("replica factor: %d", rf)
	log.Printf("chunk I/O nodes (discovered): %v", chunks.Nodes())

	capacityFn := func(ctx context.Context) uint64 {
		ov, err := apiClient.Overview(ctx)
		if err != nil {
			return 0
		}
		if ov.Storage.TotalDiskQuotaBytes > 0 {
			return uint64(ov.Storage.TotalDiskQuotaBytes)
		}
		return service.SumDiskQuotaBytes(*ov, nil)
	}
	usedFn := func(ctx context.Context) uint64 {
		ov, err := apiClient.Overview(ctx)
		if err != nil {
			return 0
		}
		if ov.Storage.TotalDiskBytes > 0 || ov.Storage.TotalMemCacheBytes > 0 {
			return uint64(ov.Storage.TotalDiskBytes + ov.Storage.TotalMemCacheBytes)
		}
		return service.SumDiskUsageBytes(*ov, nil)
	}
	if cap := capacityFn(context.Background()); cap > 0 {
		log.Printf("df capacity: %d bytes (cluster total disk_quota_bytes)", cap)
	} else {
		log.Printf("df capacity: unlimited (nodes report no disk quota)")
	}

	uid := uint32(syscall.Getuid())
	gid := uint32(syscall.Getgid())
	root := fusefs.NewRoot(metaStore, chunks, uid, gid, capacityFn, usedFn)

	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug:  *debug,
			Name:   "memoryfs",
			FsName: "memoryfs",
		},
	}

	server, err := fs.Mount(*mountPoint, root, opts)
	if err != nil {
		log.Fatalf("mount: %v", err)
	}

	log.Printf("memoryfs mounted at %s (df -h %s)", *mountPoint, *mountPoint)
	log.Printf("mount container must keep running; if it exits the host bind mount shows 'Transport endpoint is not connected'")

	if *verbose {
		go heartbeat(apiClient, chunks)
	}

	if *foreground {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigCh
			log.Printf("received %v, unmounting %s", sig, *mountPoint)
			_ = server.Unmount()
		}()
	}

	server.Wait()
	log.Printf("FUSE server exited; %s is stale until you run: fusermount -u %s", *mountPoint, *mountPoint)
}

func heartbeat(c *cli.Client, chunks *storage.ChunkStore) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := chunks.RefreshNodes(ctx); err != nil {
			mountlog.Warnf("heartbeat: refresh nodes: %v", err)
		}
		ov, err := c.Overview(ctx)
		cancel()
		if err != nil {
			log.Printf("heartbeat: cluster unreachable: %v", err)
			continue
		}
		log.Printf("heartbeat: leader=%s epoch=%d chunk_nodes=%d cluster_nodes=%d",
			ov.Leader, ov.ClusterEpoch, len(chunks.Nodes()), reachableNodes(ov))
	}
}

func reachableNodes(ov *service.ClusterOverview) int {
	n := 0
	for _, node := range ov.Nodes {
		if node.Reachable {
			n++
		}
	}
	return n
}

func detectReplicaFactor(nodes []string) int {
	if len(nodes) == 0 {
		return chunk.DefaultReplicaFactor
	}
	seed := strings.TrimRight(strings.TrimSpace(nodes[0]), "/")
	prefix := cli.DetectPrefix(context.Background(), seed, "")
	c := cli.NewClient(seed, prefix, "")
	ov, err := c.Overview(context.Background())
	if err != nil || ov.ReplicaFactor <= 0 {
		log.Printf("warning: detect replica factor: %v; using default %d", err, chunk.DefaultReplicaFactor)
		return chunk.DefaultReplicaFactor
	}
	return ov.ReplicaFactor
}

func prefixedLeaderURL(leader, prefix string) string {
	leader = strings.TrimRight(strings.TrimSpace(leader), "/")
	if prefix == "" || strings.HasSuffix(leader, prefix) {
		return leader
	}
	return leader + prefix
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
