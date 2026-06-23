package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/cli"
)

func runBenchmark(args []string) {
	fs := flag.NewFlagSet("benchmark", flag.ExitOnError)
	seed := fs.String("nodes", "", "seed node HTTP URL (default: $MEMORYFS_NODES, then saved mount config)")
	uriPrefix := fs.String("uri-prefix", "", "HTTP URI prefix (default: $MEMORYFS_URI_PREFIX, then saved mount config, then auto-detect)")
	token := fs.String("api-token", "", "API bearer token (default: $MEMORYFS_API_TOKEN, then saved mount config)")
	size := fs.Int("size", 4<<20, "chunk payload size in bytes")
	writes := fs.Int("writes", 50, "number of chunk writes")
	reads := fs.Int("reads", 50, "number of chunk reads")
	workers := fs.Int("workers", 4, "parallel workers")
	prefix := fs.String("prefix", "bench", "chunk ID prefix for test data")
	cleanup := fs.Bool("cleanup", true, "delete benchmark chunks after run")
	timeout := fs.Duration("timeout", 5*time.Minute, "overall timeout")
	_ = fs.Parse(args)

	resolved := resolveConn(*seed, *uriPrefix, *token)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if _, err := cli.RunBenchmark(ctx, cli.BenchmarkOptions{
		Seed:       resolved.Seed,
		URIPrefix:  resolved.URIPrefix,
		Token:      resolved.Token,
		AutoPrefix: resolved.URIPrefix == "",
		Size:       *size,
		Writes:     *writes,
		Reads:      *reads,
		Workers:    *workers,
		Prefix:     *prefix,
		Cleanup:    *cleanup,
	}, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "benchmark: %v\n", err)
		os.Exit(1)
	}
}
