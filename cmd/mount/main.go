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
	"github.com/shaowenchen/memoryfs/pkg/fusefs"
	"github.com/shaowenchen/memoryfs/pkg/storage"
)

func main() {
	mountPoint := flag.String("mount", "", "mount point (required)")
	nodes := flag.String("nodes", "", "comma-separated node HTTP URLs (required)")
	replicaFactor := flag.Int("replica-factor", 2, "chunk replication factor")
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

	chunks := storage.NewChunkStore(metaStore, metaStore.Nodes(), *replicaFactor)
	if err := chunks.RefreshNodes(context.Background()); err != nil {
		log.Printf("warning: refresh nodes: %v", err)
	}
	if len(chunks.Nodes()) == 0 {
		log.Printf("using configured nodes: %v", nodeList)
	} else {
		log.Printf("using nodes: %v", chunks.Nodes())
	}

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
