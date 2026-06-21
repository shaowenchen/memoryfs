package cluster

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// ReconcileRaftPeers updates raft voter addresses to match the registry (leader only).
func (m *Membership) ReconcileRaftPeers(ctx context.Context) error {
	if !m.raft.IsLeader() {
		return fmt.Errorf("not leader")
	}
	members, err := m.List(ctx)
	if err != nil {
		return err
	}
	for _, member := range members {
		if member.Raft == "" {
			continue
		}
		if err := m.ensureRaftVoter(member); err != nil {
			return err
		}
	}
	return nil
}

func (m *Membership) ensureRaftVoter(member Member) error {
	current, ok, err := m.raft.ServerAddress(member.ID)
	if err != nil {
		return err
	}
	if ok && current == member.Raft {
		return nil
	}
	if ok && isKubernetesDNS(current) && !isKubernetesDNS(member.Raft) {
		log.Printf("cluster: raft peer %s %s -> %s", member.ID, current, member.Raft)
	}
	if err := m.raft.AddVoter(member.ID, member.Raft); err != nil {
		return fmt.Errorf("raft peer %s: %w", member.ID, err)
	}
	return nil
}

func isKubernetesDNS(addr string) bool {
	return strings.Contains(addr, ".svc") || strings.Contains(addr, "cluster.local")
}
