package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/cli"
)

func main() {
	seed := flag.String("nodes", envOr("MEMORYFS_NODES", "http://127.0.0.1:8080"), "seed node HTTP URL")
	uriPrefix := flag.String("uri-prefix", envOr("MEMORYFS_URI_PREFIX", ""), "HTTP URI prefix (empty=auto detect /memoryfs)")
	token := flag.String("api-token", envOr("MEMORYFS_API_TOKEN", ""), "optional API bearer token")
	jsonOut := flag.Bool("json", false, "JSON output")
	timeout := flag.Duration("timeout", 30*time.Second, "request timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := cli.PrintStatus(ctx, cli.StatusOptions{
		Seed:       firstNode(*seed),
		URIPrefix:  *uriPrefix,
		Token:      *token,
		JSON:       *jsonOut,
		AutoPrefix: *uriPrefix == "",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "status: %v\n", err)
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
	return "http://127.0.0.1:8080"
}
