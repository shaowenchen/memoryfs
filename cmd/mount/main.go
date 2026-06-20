//go:build !windows

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/cli"
	"github.com/shaowenchen/memoryfs/pkg/client"
	"github.com/shaowenchen/memoryfs/pkg/fusefs"
	"github.com/shaowenchen/memoryfs/pkg/service"
	"github.com/shaowenchen/memoryfs/pkg/storage"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	mountPoint := flag.String("mount", "", "mount point (required)")
	nodes := flag.String("nodes", envOr("MEMORYFS_NODES", ""), "comma-separated node HTTP URLs (required)")
	sizeGB := flag.Int("size-gb", envIntOr("MEMORYFS_SIZE_GB", 32), "reported filesystem size for df (GB)")
	foreground := flag.Bool("f", false, "run in foreground")
	debug := flag.Bool("debug", false, "enable fuse debug")
	verbose := flag.Bool("v", false, "log periodic health heartbeats")
	flag.Parse()

	if *mountPoint == "" {
		log.Fatal("mount point is required: -mount /path/to/mount")
	}
	if *nodes == "" {
		log.Fatal("nodes is required: -nodes http://127.0.0.1:8080")
	}

	var nodeList []string
	for _, n := range strings.Split(*nodes, ",") {
		if n = strings.TrimSpace(n); n != "" {
			nodeList = append(nodeList, n)
		}
	}

	log.Printf("memoryfs mount starting pid=%d mount=%s nodes=%v", os.Getpid(), *mountPoint, nodeList)

	metaStore := client.NewRemoteMeta(nodeList)
	defer func() { _ = metaStore.Close() }()

	seed := strings.TrimRight(strings.TrimSpace(nodeList[0]), "/")
	prefix := cli.NormalizePrefix(cli.DetectPrefix(context.Background(), seed, ""))
	apiClient := cli.NewClient(seed, prefix, "")

	ov, err := apiClient.Overview(context.Background())
	if err != nil {
		log.Fatalf("cluster unreachable at %s: %v", seed, err)
	}
	log.Printf("cluster ok: leader=%s rf=%d epoch=%d reachable_nodes=%d",
		ov.Leader, ov.ReplicaFactor, ov.ClusterEpoch, reachableNodes(ov))

	rf := ov.ReplicaFactor
	if rf <= 0 {
		rf = detectReplicaFactor(nodeList)
	}
	chunkNodes := metaStore.Nodes()
	chunks := storage.NewHTTPChunkStore(metaStore, chunkNodes, rf)
	if err := chunks.RefreshNodes(context.Background()); err != nil {
		log.Printf("warning: refresh nodes: %v", err)
	}
	if len(chunks.Nodes()) == 0 {
		log.Fatal("no cluster nodes available; check -nodes URLs")
	}
	log.Printf("replica factor: %d", rf)
	log.Printf("using nodes: %v", chunks.Nodes())

	sizeBytes := uint64(*sizeGB) << 30
	usedFn := func(ctx context.Context) uint64 {
		latest, err := apiClient.Overview(ctx)
		if err != nil {
			return 0
		}
		return usedBytesForNodes(latest, chunkNodes)
	}

	uid := uint32(syscall.Getuid())
	gid := uint32(syscall.Getgid())
	root := fusefs.NewRoot(metaStore, chunks, uid, gid, sizeBytes, usedFn)

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
		go heartbeat(apiClient, chunkNodes)
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

func heartbeat(c *cli.Client, nodes []string) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ov, err := c.Overview(ctx)
		cancel()
		if err != nil {
			log.Printf("heartbeat: cluster unreachable: %v", err)
			continue
		}
		log.Printf("heartbeat: leader=%s epoch=%d used=%d bytes nodes=%d",
			ov.Leader, ov.ClusterEpoch, usedBytesForNodes(ov, nodes), reachableNodes(ov))
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

func usedBytesForNodes(ov *service.ClusterOverview, nodes []string) uint64 {
	want := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		want[normalizeNodeKey(n)] = struct{}{}
	}
	var used uint64
	for _, node := range ov.Nodes {
		if len(want) > 0 {
			if _, ok := want[normalizeNodeKey(node.URL)]; !ok {
				continue
			}
		}
		used += uint64(node.Stats.DiskBytes + node.Stats.MemCacheBytes)
	}
	return used
}

func normalizeNodeKey(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	if i := strings.Index(raw, "/"); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimRight(raw, "/")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return fallback
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
