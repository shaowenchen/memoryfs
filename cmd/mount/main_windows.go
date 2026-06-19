//go:build windows

package main

import "log"

func main() {
	log.Fatal("memoryfs mount requires Linux or macOS with FUSE support")
}
