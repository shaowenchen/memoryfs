package transport

import "testing"

func TestNormalizeGRPC(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"http://10.0.0.1:8080", "10.0.0.1:9090"},
		{"http://10.0.0.1:8080/memoryfs", "10.0.0.1:9090"},
		{"https://10.0.0.1:9090/memoryfs", "10.0.0.1:9090"},
		{"10.0.0.1", "10.0.0.1:9090"},
	}
	for _, tc := range tests {
		if got := normalizeGRPC(tc.in); got != tc.want {
			t.Fatalf("normalizeGRPC(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
