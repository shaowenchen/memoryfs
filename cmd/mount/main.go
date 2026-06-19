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

	"github.com/shaowenchen/memoryfs/pkg/fusefs"
	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/storage"
)

func main() {
	mountPoint := flag.String("mount", "", "mount point (required)")
	redisAddr := flag.String("redis", "127.0.0.1:6379", "redis address for metadata")
	workers := flag.String("workers", "", "comma-separated worker URLs (optional if registered in redis)")
	foreground := flag.Bool("f", false, "run in foreground")
	debug := flag.Bool("debug", false, "enable fuse debug")
	flag.Parse()

	if *mountPoint == "" {
		log.Fatal("mount point is required: -mount /path/to/mount")
	}

	store, err := meta.NewStore(*redisAddr)
	if err != nil {
		log.Fatalf("metadata store: %v", err)
	}
	defer store.Close()

	var workerList []string
	if *workers != "" {
		for _, w := range strings.Split(*workers, ",") {
			if w = strings.TrimSpace(w); w != "" {
				workerList = append(workerList, w)
			}
		}
	}
	chunks := storage.NewChunkStore(store, workerList)
	if err := chunks.RefreshWorkers(context.Background()); err != nil {
		log.Printf("warning: refresh workers: %v", err)
	}
	if len(chunks.Workers()) == 0 {
		log.Fatal("no workers available; start a worker or pass -workers")
	}
	log.Printf("using workers: %v", chunks.Workers())

	uid := uint32(syscall.Getuid())
	gid := uint32(syscall.Getgid())
	root := fusefs.NewRoot(store, chunks, uid, gid)

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
