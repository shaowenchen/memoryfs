// Package cliconfig persists the latest connection info of the local memoryfs
// CLI so that subcommands (status, benchmark, mount -nodes) can be invoked
// without re-specifying -nodes / -uri-prefix / -api-token every time.
//
// The mount subcommand writes the config after a successful FUSE mount.
// Other subcommands (status, benchmark) read it as a fallback when the user
// did not pass -nodes explicitly and MEMORYFS_NODES is empty.
//
// Default path:
//   - $MEMORYFS_CONFIG if set, otherwise
//   - $XDG_CONFIG_HOME/memoryfs/config.json, otherwise
//   - $HOME/.memoryfs/config.json
package cliconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config captures everything a CLI subcommand needs to reach the cluster.
type Config struct {
	Nodes         []string  `json:"nodes"`
	URIPrefix     string    `json:"uri_prefix,omitempty"`
	APIToken      string    `json:"api_token,omitempty"`
	MountPoint    string    `json:"mount_point,omitempty"`
	Leader        string    `json:"leader,omitempty"`
	ReplicaFactor int       `json:"replica_factor,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Path returns the resolved location used for Save/Load.
// It does not create any directory.
func Path() string {
	if p := strings.TrimSpace(os.Getenv("MEMORYFS_CONFIG")); p != "" {
		return p
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "memoryfs", "config.json")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".memoryfs", "config.json")
	}
	return filepath.Join(os.TempDir(), "memoryfs", "config.json")
}

// Save writes the config atomically to Path().
func Save(c *Config) error {
	if c == nil {
		return errors.New("cliconfig: nil config")
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now().UTC()
	}
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cliconfig: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("cliconfig: marshal: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("cliconfig: tempfile: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("cliconfig: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cliconfig: close: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("cliconfig: chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("cliconfig: rename: %w", err)
	}
	return nil
}

// Load reads the config from Path(). Returns os.ErrNotExist when no file
// has been saved yet — callers should treat that as "no fallback available".
func Load() (*Config, error) {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("cliconfig: parse %s: %w", path, err)
	}
	return cfg, nil
}

// Remove deletes the saved config (used by "memoryfs config clear").
// Returns nil if no file was present.
func Remove() error {
	if err := os.Remove(Path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
