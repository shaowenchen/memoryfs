package cliconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MEMORYFS_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("XDG_CONFIG_HOME", "")

	in := &Config{
		Nodes:         []string{"http://10.0.0.1:19800", "http://10.0.0.2:19800"},
		URIPrefix:     "/memoryfs",
		APIToken:      "token",
		MountPoint:    "/mnt/memoryfs",
		Leader:        "http://10.0.0.1:19800",
		ReplicaFactor: 2,
	}
	if err := Save(in); err != nil {
		t.Fatalf("save: %v", err)
	}

	st, err := os.Stat(Path())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Fatalf("unexpected mode %v", mode)
	}

	out, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out.Nodes) != 2 || out.Nodes[0] != "http://10.0.0.1:19800" {
		t.Fatalf("nodes mismatch: %v", out.Nodes)
	}
	if out.URIPrefix != "/memoryfs" || out.APIToken != "token" || out.MountPoint != "/mnt/memoryfs" {
		t.Fatalf("fields mismatch: %+v", out)
	}
	if out.UpdatedAt.IsZero() {
		t.Fatalf("updated_at not set")
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MEMORYFS_CONFIG", filepath.Join(dir, "missing.json"))
	if _, err := Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MEMORYFS_CONFIG", filepath.Join(dir, "config.json"))
	if err := Save(&Config{Nodes: []string{"http://10.0.0.1:19800"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := Remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file gone, got %v", err)
	}
	if err := Remove(); err != nil {
		t.Fatalf("remove (idempotent): %v", err)
	}
}

func TestPathPrecedence(t *testing.T) {
	t.Setenv("MEMORYFS_CONFIG", "/tmp/explicit.json")
	if got := Path(); got != "/tmp/explicit.json" {
		t.Fatalf("MEMORYFS_CONFIG not honored: %s", got)
	}
	t.Setenv("MEMORYFS_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got := Path(); !strings.HasSuffix(got, "/tmp/xdg/memoryfs/config.json") {
		t.Fatalf("XDG_CONFIG_HOME not honored: %s", got)
	}
}
