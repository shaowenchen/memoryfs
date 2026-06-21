package raftnode

import (
	"fmt"
	"net"
	"strings"

	"github.com/hashicorp/raft"
)

func validateExistingConfig(r *raft.Raft, logStore raft.LogStore, stableStore raft.StableStore, snapStore raft.SnapshotStore, cfg Config) error {
	hasState, err := raft.HasExistingState(logStore, stableStore, snapStore)
	if err != nil {
		return fmt.Errorf("raft state check: %w", err)
	}
	if !hasState {
		return nil
	}

	expectedPort := addrPort(cfg.AdvertiseRaft)
	if expectedPort == "" {
		expectedPort = addrPort(cfg.RaftAddr)
	}
	if expectedPort == "" {
		return nil
	}

	future := r.GetConfiguration()
	if err := future.Error(); err != nil {
		return fmt.Errorf("raft configuration: %w", err)
	}
	return validatePeerPorts(future.Configuration().Servers, expectedPort, cfg.DataDir)
}

func validatePeerPorts(servers []raft.Server, expectedPort, dataDir string) error {
	if expectedPort == "" {
		return nil
	}
	var stale []string
	for _, s := range servers {
		if peerPort := addrPort(string(s.Address)); peerPort != "" && peerPort != expectedPort {
			stale = append(stale, fmt.Sprintf("%s@%s", s.ID, s.Address))
		}
	}
	if len(stale) == 0 {
		return nil
	}
	return fmt.Errorf(
		"stale raft peer addresses [%s] (this node listens on port %s); delete %s (raft.db and snapshots/) on every cluster node, then helm uninstall and reinstall",
		strings.Join(stale, ", "),
		expectedPort,
		dataDir,
	)
}

func addrPort(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, ":") {
		return strings.TrimPrefix(addr, ":")
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}
