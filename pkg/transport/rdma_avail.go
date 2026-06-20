package transport

import (
	"os"
	"strings"
)

var rdmaDeviceDir = "/dev/infiniband"

// RDMAAvailable reports whether InfiniBand userspace devices are present.
func RDMAAvailable() bool {
	return rdmaDevicesPresent(rdmaDeviceDir)
}

func rdmaDevicesPresent(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "uverbs") {
			return true
		}
	}
	return false
}
