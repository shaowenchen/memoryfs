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

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/shaowenchen/memoryfs/pkg/client"
	"github.com/shaowenchen/memoryfs/pkg/cli"
	"github.com/shaowenchen/memoryfs/pkg/chunk"
	"github.com/shaowenchen/memoryfs/pkg/fusefs"
	"github.com/shaowenchen/memoryfs/pkg/storage"
)

func main() {
	mountPoint := flag.String("mount", "", "mount point (required)")
	nodes := flag.String("nodes", envOr("MEMORYFS_NODES", ""), "comma-separated node HTTP URLs (required)")
	foreground := flag.Bool("f", false, "run in foreground")
	debug := flag.Bool("debug", false, "enable fuse debug")
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

	metaStore := client.NewRemoteMeta(nodeList)
	defer func() { _ = metaStore.Close() }()

	rf := detectReplicaFactor(nodeList)
	chunks := storage.NewChunkStore(metaStore, nodeList, rf)
	if err := chunks.RefreshNodes(context.Background()); err != nil {
		log.Printf("warning: refresh nodes: %v", err)
	}
	if len(chunks.Nodes()) == 0 {
		log.Fatal("no cluster nodes available; check -nodes URLs")
	}
	log.Printf("replica factor: %d (from cluster)", rf)
	log.Printf("using nodes: %v", chunks.Nodes())

	uid := uint32(syscall.Getuid())
	gid := uint32(syscall.Getgid())
	root := fusefs.NewRoot(metaStore, chunks, uid, gid)

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

	log.Printf("memoryfs mounted at %s", *mountPoint)

	if *foreground {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			log.Printf("unmounting %s", *mountPoint)
			_ = server.Unmount()
		}()
	}

	server.Wait()
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
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
