package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/cli"
	"github.com/shaowenchen/memoryfs/pkg/ports"
)

func main() {
	seed := flag.String("nodes", envOr("MEMORYFS_NODES", ports.DefaultHTTPURL()), "seed node HTTP URL")
	uriPrefix := flag.String("uri-prefix", envOr("MEMORYFS_URI_PREFIX", ""), "HTTP URI prefix (empty=auto detect /memoryfs)")
	token := flag.String("api-token", envOr("MEMORYFS_API_TOKEN", ""), "optional API bearer token")
	size := flag.Int("size", 4<<20, "chunk payload size in bytes")
	writes := flag.Int("writes", 50, "number of chunk writes")
	reads := flag.Int("reads", 50, "number of chunk reads")
	workers := flag.Int("workers", 4, "parallel workers")
	prefix := flag.String("prefix", "bench", "chunk ID prefix for test data")
	cleanup := flag.Bool("cleanup", true, "delete benchmark chunks after run")
	timeout := flag.Duration("timeout", 5*time.Minute, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if _, err := cli.RunBenchmark(ctx, cli.BenchmarkOptions{
		Seed:       firstNode(*seed),
		URIPrefix:  *uriPrefix,
		Token:      *token,
		AutoPrefix: *uriPrefix == "",
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstNode(nodes string) string {
	for _, n := range strings.Split(nodes, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			return strings.TrimRight(n, "/")
		}
	}
	return ports.DefaultHTTPURL()
}
