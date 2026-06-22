package service

import "testing"

func TestLeaderAPIURL(t *testing.T) {
	got := leaderAPIURL("http://10.0.0.1:19800", "/memoryfs", "/v1/chunks/registry/set")
	want := "http://10.0.0.1:19800/memoryfs/v1/chunks/registry/set"
	if got != want {
		t.Fatalf("leaderAPIURL = %q, want %q", got, want)
	}
	got = leaderAPIURL("http://10.0.0.1:19800/", "", "/v1/chunks/registry/set")
	want = "http://10.0.0.1:19800/v1/chunks/registry/set"
	if got != want {
		t.Fatalf("leaderAPIURL no prefix = %q, want %q", got, want)
	}
}
