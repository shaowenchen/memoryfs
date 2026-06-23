package main

import "testing"

func TestNormalizeUint(t *testing.T) {
	cases := map[string]string{
		"":      "0",
		"abc":   "0",
		"12":    "12",
		" 32 ":  "32",
		"32GB":  "32",
		"v1.2":  "12",
		"01024": "01024",
	}
	for in, want := range cases {
		if got := normalizeUint(in); got != want {
			t.Errorf("normalizeUint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPortFromListen(t *testing.T) {
	cases := []struct {
		addr, fb, want string
	}{
		{":19800", "0", "19800"},
		{"0.0.0.0:19800", "0", "19800"},
		{"10.0.0.1:7000", "19800", "7000"},
		{"", "19800", "19800"},
		{"abc", "19800", "19800"},
	}
	for _, c := range cases {
		if got := portFromListen(c.addr, c.fb); got != c.want {
			t.Errorf("portFromListen(%q,%q) = %q, want %q", c.addr, c.fb, got, c.want)
		}
	}
}

func TestLastDashOrdinal(t *testing.T) {
	if got := lastDashOrdinal("memoryfs-0"); got != "0" {
		t.Fatalf("got %q", got)
	}
	if got := lastDashOrdinal("memoryfs-23"); got != "23" {
		t.Fatalf("got %q", got)
	}
	if got := lastDashOrdinal("memoryfs"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestStripLastDashSegment(t *testing.T) {
	if got := stripLastDashSegment("memoryfs-0"); got != "memoryfs" {
		t.Fatalf("got %q", got)
	}
	if got := stripLastDashSegment("my-pod-2"); got != "my-pod" {
		t.Fatalf("got %q", got)
	}
}

func TestEnvBool(t *testing.T) {
	t.Setenv("FOO_BOOL", "true")
	if !envBool("FOO_BOOL") {
		t.Fatal("expected true")
	}
	t.Setenv("FOO_BOOL", "TRUE")
	if !envBool("FOO_BOOL") {
		t.Fatal("expected true (case-insensitive)")
	}
	t.Setenv("FOO_BOOL", "0")
	if envBool("FOO_BOOL") {
		t.Fatal("expected false")
	}
	t.Setenv("FOO_BOOL", "")
	if envBool("FOO_BOOL") {
		t.Fatal("expected false (empty)")
	}
}

func TestDeriveAdvertise(t *testing.T) {
	// host-network case
	t.Setenv("MEMORYFS_HTTP_URL", "")
	t.Setenv("MEMORYFS_HOST_IP", "10.0.0.5")
	http, raft := deriveAdvertise("memoryfs-1", "memoryfs-headless", "memoryfs",
		":19800", ":19802", "19800", "19802")
	if http != "http://10.0.0.5:19800" || raft != "10.0.0.5:19802" {
		t.Fatalf("host-ip case: %s / %s", http, raft)
	}

	// headless service case
	t.Setenv("MEMORYFS_HOST_IP", "")
	http, raft = deriveAdvertise("memoryfs-2", "memoryfs-headless", "memoryfs",
		":19800", ":19802", "19800", "19802")
	wantHTTP := "http://memoryfs-2.memoryfs-headless.memoryfs.svc.cluster.local:19800"
	wantRaft := "memoryfs-2.memoryfs-headless.memoryfs.svc.cluster.local:19802"
	if http != wantHTTP || raft != wantRaft {
		t.Fatalf("headless case: %s / %s", http, raft)
	}

	// loopback case
	http, raft = deriveAdvertise("", "", "", ":19800", ":19802", "19800", "19802")
	if http != "http://127.0.0.1:19800" || raft != ":19802" {
		t.Fatalf("loopback case: %s / %s", http, raft)
	}

	// explicit HTTP URL override
	t.Setenv("MEMORYFS_HTTP_URL", "http://example.com:9000")
	http, raft = deriveAdvertise("memoryfs-0", "", "", ":19800", ":19802", "19800", "19802")
	if http != "http://example.com:9000" || raft != ":19802" {
		t.Fatalf("http-url override: %s / %s", http, raft)
	}
}
