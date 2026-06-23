//go:build windows

package main

import (
	"fmt"
	"os"
)

func runMount(_ []string) {
	fmt.Fprintln(os.Stderr, "memoryfs mount is not supported on Windows (requires FUSE)")
	os.Exit(2)
}
