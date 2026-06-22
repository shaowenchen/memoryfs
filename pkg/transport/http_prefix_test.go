package transport

import "testing"

func TestPrefixedChunkURL(t *testing.T) {
	tp := NewPrefixedHTTPTransport("/memoryfs")
	got := tp.chunkURL("http://10.0.0.1:19800", "9_0_1", false)
	want := "http://10.0.0.1:19800/memoryfs/chunks/9_0_1"
	if got != want {
		t.Fatalf("chunkURL = %q, want %q", got, want)
	}
	got = tp.chunkURL("http://10.0.0.1:19800", "9_0_1", true)
	want = "http://10.0.0.1:19800/memoryfs/chunks/9_0_1?replica=1"
	if got != want {
		t.Fatalf("chunkURL replica = %q, want %q", got, want)
	}
}
