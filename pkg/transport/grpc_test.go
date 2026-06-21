package transport

import (
	"testing"

	"github.com/shaowenchen/memoryfs/pkg/ports"
)

func TestNormalizeGRPC(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"http://127.0.0.1:" + ports.HTTP, "127.0.0.1:" + ports.GRPC},
		{"http://127.0.0.1:" + ports.HTTP + "/memoryfs", "127.0.0.1:" + ports.GRPC},
		{"https://127.0.0.1:" + ports.GRPC + "/memoryfs", "127.0.0.1:" + ports.GRPC},
		{"127.0.0.1", "127.0.0.1:" + ports.GRPC},
	}
	for _, tc := range tests {
		if got := normalizeGRPC(tc.in); got != tc.want {
			t.Fatalf("normalizeGRPC(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
