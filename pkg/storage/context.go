package storage

import (
	"context"
	"time"
)

const ioTimeout = 300 * time.Second

// DetachIOContext returns a context for remote chunk/meta I/O.
// FUSE can cancel the request context before HTTP completes; detached I/O keeps running.
func DetachIOContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), ioTimeout)
}
