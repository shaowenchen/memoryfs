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

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	seed := fs.String("nodes", "", "seed node HTTP URL (default: $MEMORYFS_NODES, then saved mount config, then http://127.0.0.1:19800)")
	uriPrefix := fs.String("uri-prefix", "", "HTTP URI prefix (default: $MEMORYFS_URI_PREFIX, then saved mount config, then auto-detect)")
	token := fs.String("api-token", "", "API bearer token (default: $MEMORYFS_API_TOKEN, then saved mount config)")
	jsonOut := fs.Bool("json", false, "JSON output")
	timeout := fs.Duration("timeout", 30*time.Second, "request timeout")
	_ = fs.Parse(args)

	resolved := resolveConn(*seed, *uriPrefix, *token)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := cli.PrintStatus(ctx, cli.StatusOptions{
		Seed:       resolved.Seed,
		URIPrefix:  resolved.URIPrefix,
		Token:      resolved.Token,
		JSON:       *jsonOut,
		AutoPrefix: resolved.URIPrefix == "",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "status: %v\n", err)
		os.Exit(1)
	}
}

func firstNode(nodes string) string {
	for _, n := range strings.Split(nodes, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			return strings.TrimRight(n, "/")
		}
	}
	return ""
}
