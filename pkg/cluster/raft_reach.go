package cluster

import (
	"context"
	"fmt"
	"net"
	"time"
)

func waitRaftTCP(ctx context.Context, addr string, timeout time.Duration) error {
	if addr == "" {
		return fmt.Errorf("empty raft address")
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		d := net.Dialer{Timeout: 2 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timeout")
	}
	return fmt.Errorf("raft %s unreachable: %w", addr, lastErr)
}
