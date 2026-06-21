package raftnode

import (
	"testing"

	"github.com/hashicorp/raft"
)

func TestConfigureRaftTiming(t *testing.T) {
	cfg := raft.DefaultConfig()
	configureRaft(cfg)
	if cfg.HeartbeatTimeout != raftHeartbeatTimeout {
		t.Fatalf("heartbeat: got %v want %v", cfg.HeartbeatTimeout, raftHeartbeatTimeout)
	}
	if cfg.ElectionTimeout != raftElectionTimeout {
		t.Fatalf("election: got %v want %v", cfg.ElectionTimeout, raftElectionTimeout)
	}
	if cfg.ElectionTimeout < cfg.HeartbeatTimeout {
		t.Fatal("election timeout must be >= heartbeat timeout")
	}
	if cfg.LogLevel != "INFO" {
		t.Fatalf("log level: got %q", cfg.LogLevel)
	}
}
