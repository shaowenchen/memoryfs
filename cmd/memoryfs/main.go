// Command memoryfs is the single binary for the MemoryFS distributed in-memory
// filesystem. It dispatches to one of the following subcommands:
//
//	memoryfs node       run a node (Raft + chunk store + HTTP/gRPC API)
//	memoryfs mount      mount the filesystem via FUSE (Linux only)
//	memoryfs status     print cluster status
//	memoryfs benchmark  run a chunk read/write throughput test
//	memoryfs version    print build info
package main

import (
	"fmt"
	"os"
)

const usage = `MemoryFS — distributed in-memory filesystem

Usage:
  memoryfs <command> [flags]

Commands:
  node       run a memoryfs node (Raft + chunk store + HTTP/gRPC API)
  mount      mount the filesystem via FUSE
  status     print cluster status
  benchmark  run a chunk read/write throughput test
  config     show / clear saved CLI connection info
  version    print build info

After a successful "mount", connection info (nodes, uri-prefix, api-token)
is persisted so "status" / "benchmark" can be run without re-typing -nodes.

Run "memoryfs <command> -h" for command-specific flags.
`

// Version is populated at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "node":
		runNode(args)
	case "mount":
		runMount(args)
	case "status":
		runStatus(args)
	case "benchmark":
		runBenchmark(args)
	case "config":
		runConfig(args)
	case "version", "-v", "--version":
		fmt.Println("memoryfs", Version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
