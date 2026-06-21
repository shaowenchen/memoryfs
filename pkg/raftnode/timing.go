package raftnode

import (
	"time"

	"github.com/hashicorp/raft"
)

// Raft timing follows etcd-style production defaults (via hashicorp/raft):
//   - Leader sends heartbeats at ~HeartbeatTimeout/10 (~100ms)
//   - Follower starts election after ~HeartbeatTimeout (~1s) without leader contact
//
// HeartbeatTimeout must not be too low (election storm on jitter) or too high
// (slow failover). ElectionTimeout must be >= HeartbeatTimeout.
const (
	raftHeartbeatTimeout = 1000 * time.Millisecond
	raftElectionTimeout  = 1000 * time.Millisecond
)

func configureRaft(cfg *raft.Config) {
	cfg.HeartbeatTimeout = raftHeartbeatTimeout
	cfg.ElectionTimeout = raftElectionTimeout
	cfg.LogLevel = "INFO"
}
